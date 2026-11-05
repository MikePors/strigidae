package plugins

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// effectDone ends a run of a service. It is never returned as an effect.
const effectDone = "done"

// service is one long-running process that serves the runs of every plugin
// naming it. A request goes to its standard input as one JSON line with an
// "id"; each effect line it prints names the run with the same "id", and
// "done" ends that run. Its runs don't wait for each other.
type service struct {
	name  string
	onRun []string

	mutex   sync.Mutex
	process *serviceProcess // nil until started, and again once it ended
	next    int             // the last run identifier handed out
}

// serviceProcess is one start of a service.
type serviceProcess struct {
	input   *os.File
	writing chan struct{} // one request line at a time, acquired with the run's context
	child   *os.Process
	// pending are the runs waiting for "done", by identifier. Guarded by the
	// service's mutex.
	pending map[string]*pendingRun
}

// pendingRun gathers the effects of one run until "done".
type pendingRun struct {
	effects []Effect
	size    int               // bytes of effect lines so far
	reply   chan serviceReply // buffered: written once, by whoever ends the run
}

type serviceReply struct {
	effects []Effect
	err     error
}

// ask sends the request to the plugin's service, starting it if it isn't
// running, and waits for the run's "done". The timeout starts here, with the
// run. After a complete request, a timeout or cancel leaves the service
// running; a partial request retires it so the next run has a clean stream.
func (runner *Runner) ask(ctx context.Context, found *entry, request Request) ([]Effect, error) {
	name := found.plugin.Command
	runContext, cancel := runner.withTimeout(ctx, found.plugin.Timeout)
	defer cancel()

	process, identifier, reply, err := runner.begin(found.service)
	if err != nil {
		return nil, fmt.Errorf("start plugin %q: %w", name, err)
	}
	defer found.service.forget(process, identifier)
	line, err := json.Marshal(struct {
		Identifier string `json:"id"`
		Request
	}{identifier, request})
	if err != nil {
		return nil, fmt.Errorf("encode request for plugin %q: %w", name, err)
	}
	line = append(line, '\n')
	if err := found.service.write(runContext, process, line); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, fmt.Errorf("plugin %q: %w after %s", name, ErrTimeout, found.plugin.Timeout)
		}
		if errors.Is(err, syscall.EPIPE) || !found.service.isCurrent(process) {
			// The process ended, or was retired, since the run joined it; its
			// input may be closed or broken.
			return nil, fmt.Errorf("plugin %q: %w", name, ErrExited)
		}
		return nil, fmt.Errorf("write request to plugin %q: %w", name, err)
	}
	runner.sent(name)
	select {
	case answered := <-reply:
		if answered.err != nil {
			return nil, fmt.Errorf("plugin %q: %w", name, answered.err)
		}
		return answered.effects, nil
	case <-runContext.Done():
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("plugin %q: %w after %s", name, ErrTimeout, found.plugin.Timeout)
}

// begin hands out a run of the service, starting the process if needed.
func (runner *Runner) begin(serving *service) (*serviceProcess, string, <-chan serviceReply, error) {
	serving.mutex.Lock()
	defer serving.mutex.Unlock()
	if serving.process == nil {
		process, err := runner.startService(serving)
		if err != nil {
			return nil, "", nil, err
		}
		serving.process = process
	}
	serving.next++
	identifier := strconv.Itoa(serving.next)
	waiting := &pendingRun{reply: make(chan serviceReply, 1)}
	serving.process.pending[identifier] = waiting
	return serving.process, identifier, waiting.reply, nil
}

// forget drops a run that no longer waits, so later lines for it are dropped.
func (serving *service) forget(process *serviceProcess, identifier string) {
	serving.mutex.Lock()
	defer serving.mutex.Unlock()
	delete(process.pending, identifier)
}

// isCurrent reports whether the process is the one new runs join.
func (serving *service) isCurrent(process *serviceProcess) bool {
	serving.mutex.Lock()
	defer serving.mutex.Unlock()
	return serving.process == process
}

// retire prevents new runs from using a broken stream and ends its process.
func (serving *service) retire(process *serviceProcess) {
	serving.mutex.Lock()
	if serving.process == process {
		serving.process = nil
	}
	serving.mutex.Unlock()
	_ = process.child.Kill() // it may have exited already
}

// write sends one request line. Both waiting for another writer and writing
// to a full pipe stop when the run is cancelled or its deadline expires.
func (serving *service) write(ctx context.Context, process *serviceProcess, line []byte) error {
	select {
	case process.writing <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-process.writing }()
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, _ := ctx.Deadline() // the zero time is no deadline
	if err := process.input.SetWriteDeadline(deadline); err != nil {
		serving.retire(process) // the input is closed or cannot time out
		return err
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = process.input.SetWriteDeadline(time.Now())
		close(interrupted)
	})
	defer func() {
		// The callback must end before the next writer sets its deadline.
		if !stop() {
			<-interrupted
		}
	}()
	written, err := process.input.Write(line)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	// Retire before releasing the writer, so another request cannot append
	// to an unfinished line. Cancellation before any bytes leaves it usable.
	interruptedWrite := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
	if err != nil && (written > 0 && written < len(line) || !interruptedWrite) {
		serving.retire(process)
	}
	return err
}

// startService starts the process. It is called with the service's mutex
// held. The process lives until it exits or Stop kills it, in a session of
// its own like a run's.
func (runner *Runner) startService(serving *service) (*serviceProcess, error) {
	program, err := runner.program(serving.onRun[0])
	if err != nil {
		return nil, err
	}
	// Counted before it starts, so Stop waits for it; none starts after Stop.
	runner.stopping.Lock()
	if runner.stopped {
		runner.stopping.Unlock()
		return nil, errors.New("strig is exiting")
	}
	runner.serviceProcesses.Add(1)
	runner.stopping.Unlock()
	started := false
	defer func() {
		if !started {
			runner.serviceProcesses.Done()
		}
	}()
	inputReader, inputWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outputReader, outputWriter, err := os.Pipe()
	if err != nil {
		return nil, errors.Join(err, inputReader.Close(), inputWriter.Close())
	}
	command := exec.CommandContext(runner.serviceContext, program, serving.onRun[1:]...)
	command.Env = runner.environment
	command.Stdin, command.Stdout = inputReader, outputWriter
	standardError, flushLog := runner.standardErrorSink(serving.name)
	command.Stderr = &boundedLog{writer: standardError, remaining: MaximumErrorOutputSize, cutOff: func() {
		flushLog() // the cut line comes before the note about it
		runner.logger.Warn("plugin standard error cut off", "plugin", serving.name, "limit", MaximumErrorOutputSize)
	}}
	command.WaitDelay = runner.waitDelay
	detach(command)
	startError := command.Start()
	// The child has its own copies; strig keeps only its ends.
	closeError := errors.Join(inputReader.Close(), outputWriter.Close())
	if startError != nil {
		return nil, errors.Join(startError, inputWriter.Close(), outputReader.Close())
	}
	if closeError != nil {
		runner.logger.Warn("close the service's pipe ends", "plugin", serving.name, "err", closeError)
	}
	process := &serviceProcess{input: inputWriter, writing: make(chan struct{}, 1), child: command.Process, pending: map[string]*pendingRun{}}
	read := make(chan error, 1)
	started = true
	go func() { read <- runner.readService(serving, process, outputReader, command) }()
	go runner.waitService(serving, process, command, outputReader, read, flushLog)
	return process, nil
}

// readService reads the effect lines until the output ends. A line past the
// output limit can't be read, so it kills the process.
func (runner *Runner) readService(serving *service, process *serviceProcess, output *os.File, command *exec.Cmd) error {
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 0, 64<<10), MaximumOutputSize)
	for scanner.Scan() {
		runner.serviceLine(serving, process, scanner.Bytes())
	}
	err := scanner.Err()
	if errors.Is(err, bufio.ErrTooLong) {
		runner.logger.Warn("plugin service printed a line over the limit; killed", "plugin", serving.name, "limit", MaximumOutputSize)
		_ = command.Process.Kill() // it may have exited already
		return ErrOutputTooLarge
	}
	return err
}

// serviceLine handles one line. A line naming no run under way is logged and
// dropped, so a service survives bad output; a bad effect of a run fails that
// run, as it fails a one-off run.
func (runner *Runner) serviceLine(serving *service, process *serviceProcess, line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	var header struct {
		Identifier *string `json:"id"`
		Name       string  `json:"name"`
	}
	if err := json.Unmarshal(line, &header); err != nil || header.Identifier == nil {
		runner.logger.Warn("plugin service printed a line that is not an effect of a run; skipped", "plugin", serving.name)
		return
	}
	serving.mutex.Lock()
	defer serving.mutex.Unlock()
	identifier := *header.Identifier
	waiting, ok := process.pending[identifier]
	if !ok {
		runner.logger.Info("plugin service printed an effect for no run under way; dropped", "plugin", serving.name, "effect", header.Name)
		return
	}
	end := func(reply serviceReply) {
		delete(process.pending, identifier)
		waiting.reply <- reply
	}
	if header.Name == effectDone {
		end(serviceReply{effects: waiting.effects})
		return
	}
	waiting.size += len(line)
	if waiting.size > MaximumOutputSize {
		end(serviceReply{err: fmt.Errorf("%w (limit %d bytes)", ErrOutputTooLarge, MaximumOutputSize)})
		return
	}
	effect, known, err := decodeEffect(line)
	switch {
	case err != nil:
		end(serviceReply{err: ErrInvalidOutput})
	case !known:
		runner.skippedEffect(serving.name, effect.Name)
	default:
		waiting.effects = append(waiting.effects, effect)
	}
}

// waitService waits for the process to end, then fails its runs still under
// way, so the next run starts it again.
func (runner *Runner) waitService(serving *service, process *serviceProcess, command *exec.Cmd, output *os.File, read <-chan error, flushLog func()) {
	defer runner.serviceProcesses.Done()
	waitError := command.Wait()
	// A new run must not join this process while its old output drains.
	serving.mutex.Lock()
	if serving.process == process {
		serving.process = nil
	}
	serving.mutex.Unlock()
	runner.exited(serving.name)
	// Something the service started can hold its output open; reading stops
	// a moment after the service itself ended.
	var readError error
	select {
	case readError = <-read:
	case <-time.After(runner.waitDelay):
		_ = output.SetReadDeadline(time.Now())
		readError = <-read
	}
	flushLog()
	runner.logger.Info("plugin service ended", "plugin", serving.name, "err", waitError)
	reason := ErrExited
	if errors.Is(readError, ErrOutputTooLarge) {
		reason = fmt.Errorf("%w (limit %d bytes)", ErrOutputTooLarge, MaximumOutputSize)
	}

	serving.mutex.Lock()
	defer serving.mutex.Unlock()
	for identifier, waiting := range process.pending {
		delete(process.pending, identifier)
		waiting.reply <- serviceReply{err: reason}
	}
	if serving.process == process {
		serving.process = nil
	}
	if err := errors.Join(output.Close(), process.input.Close()); err != nil {
		runner.logger.Warn("close the service's pipes", "plugin", serving.name, "err", err)
	}
}

// Stop kills the services' processes and returns once they have ended; runs
// waiting on one fail. Model.Close calls it as strig exits, alongside run
// cancellation and file flushing. A run after Stop can't start a service.
func (runner *Runner) Stop() {
	runner.stopping.Lock()
	runner.stopped = true
	runner.stopping.Unlock()
	runner.stopServices()
	runner.serviceProcesses.Wait()
}
