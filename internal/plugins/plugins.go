// Package plugins runs the programs behind the user's configured command
// plugins. A run starts the program, writes one JSON request to its standard
// input and reads effects, JSON lines on its standard output, that say what
// the plugin would like strig to do. A plugin suggests; strig decides, so a
// plugin gets no GitHub token and cannot write to GitHub.
//
// The package is non-UI and has no Bubble Tea import. Run blocks, so a caller
// puts it inside a tea.Cmd.
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/environment"
)

// Protocol is the version written as "protocol" in every request.
const Protocol = 1

// MaximumOutputSize is the most standard output one run may print. Past it
// the process is killed and the run fails.
const MaximumOutputSize = 1 << 20

// DefaultWaitDelay is how long strig keeps reading after the plugin process
// exited. Something the plugin started can hold standard output open for as
// long as it likes; once this has passed strig stops reading.
const DefaultWaitDelay = time.Second

// Effect names a plugin may print.
const (
	// EffectNotice asks for a short message in the status bar.
	EffectNotice = "notice"
	// EffectError reports a failure; the text is the reason.
	EffectError = "error"
	// EffectReload asks strig to reload what is on screen.
	EffectReload = "reload"
	// EffectAnswer carries an answer to show to the user.
	EffectAnswer = "answer"
)

// Errors a run can fail with, to be tested with errors.Is.
var (
	// ErrUnknownPlugin means no plugin has the requested name.
	ErrUnknownPlugin = errors.New("no such plugin")
	// ErrFailed means the plugin exited with a failure status.
	ErrFailed = errors.New("plugin failed")
	// ErrTimeout means the plugin ran past its timeout and was killed.
	ErrTimeout = errors.New("plugin timed out")
	// ErrOutputTooLarge means the plugin printed more than MaximumOutputSize
	// and was killed.
	ErrOutputTooLarge = errors.New("plugin printed too much output")
	// ErrInvalidOutput means a line the plugin printed is not an effect.
	ErrInvalidOutput = errors.New("plugin printed a line that is not an effect")
)

// Request is the JSON object a plugin reads from standard input. The fields
// that describe what the user is pointing at are defined per target by the
// slice that registers the commands.
type Request struct {
	// Protocol is set by the Runner; whatever the caller puts here is replaced.
	Protocol int `json:"protocol"`
}

// Effect is one thing a plugin asked for. Text is empty for effects that have
// none. It is untrusted: sanitise it where it is shown.
type Effect struct {
	Name string
	Text string
}

// Options are the inputs a Runner takes instead of reading globals.
type Options struct {
	// Environment is the environment in os.Environ form. The GitHub token
	// variables are removed before a plugin sees it.
	Environment []string
	// Log receives the plugins' standard error and strig's notes about their
	// output. Nil discards it. It is never given a request.
	Log io.Writer
	// WaitDelay overrides DefaultWaitDelay when positive.
	WaitDelay time.Duration
}

// Runner runs the configured plugins, one run at a time per plugin.
type Runner struct {
	entries     map[string]*entry
	environment []string
	log         io.Writer
	waitDelay   time.Duration

	// Seams for tests: called when a run joins a queue, and to start a run's
	// timeout.
	queued      func(name string)
	withTimeout func(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc)
}

type entry struct {
	plugin config.Plugin
	queue  queue
}

// New returns a Runner for the plugins, which config has already validated.
func New(plugins []config.Plugin, options Options) *Runner {
	runner := &Runner{
		entries:     make(map[string]*entry, len(plugins)),
		environment: environment.Scrub(options.Environment),
		log:         &lockedWriter{writer: io.Discard},
		waitDelay:   DefaultWaitDelay,
		queued:      func(string) {},
		withTimeout: context.WithTimeout,
	}
	if options.Log != nil {
		runner.log = &lockedWriter{writer: options.Log}
	}
	if options.WaitDelay > 0 {
		runner.waitDelay = options.WaitDelay
	}
	for _, plugin := range plugins {
		runner.entries[plugin.Command] = &entry{plugin: plugin}
	}
	return runner
}

// Run runs the plugin called name with the request and returns its effects in
// the order it printed them. If another run of the same plugin is under way, it
// waits its turn; cancelling ctx while waiting leaves the queue without
// running anything. The plugin's timeout starts when the run starts.
//
// A run fails, with no effects, if the plugin cannot be started, times out,
// prints too much or a line that is not an effect, or exits with a failure
// status. An error effect is an ordinary effect: a run can return one and
// succeed. If the plugin exits with a failure status after printing an error
// effect, the error carries that effect's text.
func (runner *Runner) Run(ctx context.Context, name string, request Request) ([]Effect, error) {
	request.Protocol = Protocol
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode request for plugin %q: %w", name, err)
	}
	found, ok := runner.entries[name]
	if !ok {
		return nil, fmt.Errorf("plugin %q: %w", name, ErrUnknownPlugin)
	}
	return runner.execute(ctx, found, body)
}

func (runner *Runner) execute(ctx context.Context, found *entry, body []byte) ([]Effect, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := found.plugin.Command
	if err := found.queue.acquire(ctx, func() { runner.queued(name) }); err != nil {
		return nil, err
	}
	defer found.queue.release()

	runContext, cancel := runner.withTimeout(ctx, found.plugin.Timeout)
	defer cancel()
	return runner.start(ctx, runContext, found.plugin, body)
}

// start runs the process. parent is the caller's context, runContext also has
// the timeout; comparing them tells a timeout from a cancellation.
func (runner *Runner) start(parent, runContext context.Context, plugin config.Plugin, body []byte) ([]Effect, error) {
	stopContext, stop := context.WithCancel(runContext)
	defer stop()

	output := &limitedBuffer{limit: MaximumOutputSize, stop: stop}
	command := exec.CommandContext(stopContext, plugin.OnRun[0], plugin.OnRun[1:]...)
	command.Env = runner.environment
	// A plugin that never reads its input gets EPIPE on the write; exec
	// ignores that, and the exit status decides.
	command.Stdin = bytes.NewReader(body)
	command.Stdout = output
	command.Stderr = runner.log
	command.WaitDelay = runner.waitDelay

	waitError := command.Run()

	switch {
	case output.exceeded():
		return nil, fmt.Errorf("plugin %q: %w (limit %d bytes)", plugin.Command, ErrOutputTooLarge, MaximumOutputSize)
	case parent.Err() != nil:
		return nil, parent.Err()
	case runContext.Err() != nil:
		return nil, fmt.Errorf("plugin %q: %w after %s", plugin.Command, ErrTimeout, plugin.Timeout)
	}

	var exitError *exec.ExitError
	switch {
	case waitError == nil, errors.Is(waitError, exec.ErrWaitDelay):
		// ErrWaitDelay is only reported when the plugin itself exited 0; what
		// it printed before is complete.
		effects, err := runner.parse(plugin.Command, output.bytes())
		if err != nil {
			return nil, err
		}
		return effects, nil
	case errors.As(waitError, &exitError):
		return nil, runner.exitFailure(plugin.Command, output.bytes())
	default:
		return nil, fmt.Errorf("start plugin %q: %w", plugin.Command, waitError)
	}
}

// exitFailure is the error for a plugin that exited with a failure status:
// the text of its error effect if it printed one, else a fixed message.
func (runner *Runner) exitFailure(name string, output []byte) error {
	effects, _ := runner.parse(name, output)
	for _, effect := range effects {
		if effect.Name == EffectError {
			return fmt.Errorf("plugin %q: %w", name, &reportedError{text: effect.Text})
		}
	}
	return fmt.Errorf("plugin %q: %w", name, ErrFailed)
}

// reportedError is a failure whose text the plugin chose.
type reportedError struct{ text string }

func (reported *reportedError) Error() string { return reported.text }

func (reported *reportedError) Unwrap() error { return ErrFailed }

// parse reads the effect lines in output. On a bad line it returns the effects
// before it and ErrInvalidOutput.
func (runner *Runner) parse(name string, output []byte) ([]Effect, error) {
	var effects []Effect
	for number, line := range bytes.Split(output, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		effect, known, err := decodeEffect(line)
		if err != nil {
			return effects, fmt.Errorf("plugin %q: line %d: %w", name, number+1, ErrInvalidOutput)
		}
		if !known {
			_, _ = fmt.Fprintf(runner.log, "plugin %q: skipping unknown effect %q\n", name, effect.Name)
			continue
		}
		effects = append(effects, effect)
	}
	return effects, nil
}

// decodeEffect decodes one line. known is false for an effect name this
// version does not have; a newer plugin can print those.
func decodeEffect(line []byte) (effect Effect, known bool, err error) {
	var decoded struct {
		Name string          `json:"name"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(line, &decoded); err != nil {
		return Effect{}, false, err
	}
	if decoded.Name == "" {
		return Effect{}, false, errors.New("effect has no name")
	}
	effect.Name = decoded.Name
	switch decoded.Name {
	case EffectReload:
		return effect, true, nil
	case EffectNotice, EffectError, EffectAnswer:
		var data struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(decoded.Data, &data); err != nil {
			return Effect{}, false, err
		}
		effect.Text = data.Text
		return effect, true, nil
	default:
		return effect, false, nil
	}
}

// queue lets one run at a time hold a plugin and hands it to the waiting runs
// in the order they arrived.
type queue struct {
	mutex   sync.Mutex
	busy    bool
	waiting []chan struct{}
}

// acquire waits for the plugin to be free. joined is called, without the lock,
// once the run is in the line. It returns ctx.Err() if ctx ends first, having
// left the line.
func (line *queue) acquire(ctx context.Context, joined func()) error {
	line.mutex.Lock()
	if !line.busy {
		line.busy = true
		line.mutex.Unlock()
		return nil
	}
	turn := make(chan struct{})
	line.waiting = append(line.waiting, turn)
	line.mutex.Unlock()
	joined()

	select {
	case <-turn:
		return nil
	case <-ctx.Done():
	}
	line.mutex.Lock()
	select {
	case <-turn:
		// The turn arrived as the context ended: pass it on.
		line.mutex.Unlock()
		line.release()
	default:
		for i, waiter := range line.waiting {
			if waiter == turn {
				line.waiting = append(line.waiting[:i], line.waiting[i+1:]...)
				break
			}
		}
		line.mutex.Unlock()
	}
	return ctx.Err()
}

// release hands the plugin to the next waiting run, or frees it.
func (line *queue) release() {
	line.mutex.Lock()
	defer line.mutex.Unlock()
	if len(line.waiting) == 0 {
		line.busy = false
		return
	}
	next := line.waiting[0]
	line.waiting = line.waiting[1:]
	close(next)
}

// limitedBuffer keeps a plugin's output up to a limit. Past it, it stops the
// process and refuses more, so output is never buffered without bound.
type limitedBuffer struct {
	mutex    sync.Mutex
	buffer   bytes.Buffer
	limit    int
	stop     context.CancelFunc
	overflow bool
}

var errOutputLimit = errors.New("output limit reached")

func (limited *limitedBuffer) Write(data []byte) (int, error) {
	limited.mutex.Lock()
	defer limited.mutex.Unlock()
	if limited.overflow || limited.buffer.Len()+len(data) > limited.limit {
		limited.overflow = true
		limited.stop()
		return 0, errOutputLimit
	}
	return limited.buffer.Write(data)
}

func (limited *limitedBuffer) exceeded() bool {
	limited.mutex.Lock()
	defer limited.mutex.Unlock()
	return limited.overflow
}

func (limited *limitedBuffer) bytes() []byte {
	limited.mutex.Lock()
	defer limited.mutex.Unlock()
	return bytes.Clone(limited.buffer.Bytes())
}

// lockedWriter serialises writes: the standard error of several plugins and
// strig's own notes share one log.
type lockedWriter struct {
	mutex  sync.Mutex
	writer io.Writer
}

func (locked *lockedWriter) Write(data []byte) (int, error) {
	locked.mutex.Lock()
	defer locked.mutex.Unlock()
	return locked.writer.Write(data)
}
