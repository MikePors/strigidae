//go:build unix

package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestServicePeer runs in a child test process, with real stdin/stdout pipes.
// "block" acknowledges one request, then stops reading until signalled.
func TestServicePeer(test *testing.T) {
	mode := os.Getenv("STRIG_TEST_SERVICE")
	if mode == "" {
		test.Skip("helper process")
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR1)
	if mode == "hold-output" {
		// Tell the parent that SIGUSR1 is now handled; before Notify the
		// signal would be dropped. File descriptor 3 is the first ExtraFiles entry.
		if _, err := os.NewFile(3, "ready").Write([]byte{1}); err != nil {
			os.Exit(8)
		}
		<-signals
		os.Exit(0)
	}
	directory := os.Getenv("FAKE_DIRECTORY")
	starts, err := os.OpenFile(filepath.Join(directory, "starts"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(2)
	}
	if _, err := fmt.Fprintln(starts, os.Getpid()); err != nil {
		os.Exit(2)
	}
	if err := starts.Close(); err != nil {
		os.Exit(2)
	}
	encoder := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	for scanner.Scan() {
		var request struct {
			Identifier string `json:"id"`
			Arguments  string `json:"arguments"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(3)
		}
		if request.Arguments == "exit-with-open-output" {
			child := exec.Command(os.Args[0], "-test.run=^TestServicePeer$")
			child.Env = append(os.Environ(), "STRIG_TEST_SERVICE=hold-output")
			child.Stdout = os.Stdout
			ready, readyWriter, err := os.Pipe()
			if err != nil {
				os.Exit(4)
			}
			child.ExtraFiles = []*os.File{readyWriter}
			err = child.Start()
			// Close our copy so a holder that dies early shows up as EOF.
			_ = readyWriter.Close()
			if err != nil {
				os.Exit(4)
			}
			// The test signals the holder as soon as it reads output-holder, so
			// publish the PID only once the holder is listening.
			if _, err := ready.Read(make([]byte, 1)); err != nil {
				os.Exit(9)
			}
			_ = ready.Close()
			if err := os.WriteFile(filepath.Join(directory, "output-holder"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
				os.Exit(5)
			}
			os.Exit(0)
		}
		if err := encoder.Encode(map[string]any{"id": request.Identifier, "name": "answer", "data": map[string]string{"text": request.Arguments}}); err != nil {
			os.Exit(7)
		}
		if err := encoder.Encode(map[string]string{"id": request.Identifier, "name": "done"}); err != nil {
			os.Exit(7)
		}
		if request.Arguments == "block" {
			<-signals
			var first [1]byte
			if _, err := os.Stdin.Read(first[:]); err != nil {
				os.Exit(6)
			}
			if _, err := fmt.Fprintln(os.Stderr, "service request byte read"); err != nil {
				os.Exit(7)
			}
			<-signals
		}
	}
	os.Exit(0)
}

func backpressureHarness(test *testing.T) *serviceHarness {
	test.Helper()
	harness := newServiceHarness(test, time.Minute)
	harness.runner.entries["ask"].service.onRun = []string{os.Args[0], "-test.run=^TestServicePeer$"}
	harness.runner.environment = append(harness.runner.environment, "STRIG_TEST_SERVICE=answer", "GORACE=atexit_sleep_ms=0")
	return harness
}

// A run cancelled during a partial write must return before its minute-long
// timeout, retire that stream, and let the next run start a fresh service.
func TestServiceCancelledPartialWriteRestarts(test *testing.T) {
	harness := backpressureHarness(test)
	read := make(chan struct{})
	harness.runner.logger = slog.New(slog.NewTextHandler(&requestReadLog{lockedBuffer: harness.log, read: read}, nil))
	if result := harness.run(context.Background(), "ask", "block"); result.err != nil {
		test.Fatal(result.err)
	}
	serving := harness.runner.entries["ask"].service
	serving.mutex.Lock()
	process := serving.process
	serving.mutex.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan outcome, 1)
	go func() { results <- harness.run(ctx, "ask", strings.Repeat("x", 1<<20)) }()
	if err := process.child.Signal(syscall.SIGUSR1); err != nil {
		test.Fatal(err)
	}
	// The helper reads one byte, then stops again. This proves the blocked
	// request has written part of its line before it is cancelled.
	receive(test, read)
	cancel()
	result := receive(test, results)
	if !errors.Is(result.err, context.Canceled) {
		test.Fatalf("cancelled run: %v", result.err)
	}
	if result := harness.run(context.Background(), "ask", "next"); result.err != nil || len(result.effects) != 1 || result.effects[0].Text != "next" {
		test.Errorf("run after interrupted request: %v, %v", result.effects, result.err)
	}
	if starts := harness.starts(test); len(starts) != 2 {
		test.Errorf("%d starts, want a fresh stream", len(starts))
	}
	// The retired process is reaped by Stop, even though another is active.
	harness.runner.Stop()
	if err := process.child.Signal(syscall.Signal(0)); !errors.Is(err, os.ErrProcessDone) {
		test.Errorf("retired process still alive: %v", err)
	}
}

type requestReadLog struct {
	*lockedBuffer
	read chan struct{}
	once sync.Once
}

func (log *requestReadLog) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "service request byte read") {
		log.once.Do(func() { close(log.read) })
	}
	return log.lockedBuffer.Write(data)
}

// expiredContext lets the injected timeout expire after the helper has read
// a byte. A short real timer could expire during JSON encoding on a busy host,
// before any write, which correctly leaves the service running.
type expiredContext struct{ context.Context }

func (ctx expiredContext) Err() error {
	if ctx.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}

func TestServiceTimedOutPartialWriteRestarts(test *testing.T) {
	harness := backpressureHarness(test)
	read := make(chan struct{})
	harness.runner.logger = slog.New(slog.NewTextHandler(&requestReadLog{lockedBuffer: harness.log, read: read}, nil))
	if result := harness.run(context.Background(), "ask", "block"); result.err != nil {
		test.Fatal(result.err)
	}
	serving := harness.runner.entries["ask"].service
	serving.mutex.Lock()
	process := serving.process
	serving.mutex.Unlock()
	timed, expire := context.WithTimeout(context.Background(), time.Minute)
	defer expire()
	harness.runner.withTimeout = func(context.Context, time.Duration) (context.Context, context.CancelFunc) {
		return expiredContext{timed}, expire
	}
	results := make(chan outcome, 1)
	go func() { results <- harness.run(context.Background(), "ask", strings.Repeat("x", 1<<20)) }()
	if err := process.child.Signal(syscall.SIGUSR1); err != nil {
		test.Fatal(err)
	}
	receive(test, read)
	expire()
	if result := receive(test, results); !errors.Is(result.err, ErrTimeout) {
		test.Fatalf("full pipe: %v", result.err)
	}
	harness.runner.withTimeout = context.WithTimeout
	next, cancelNext := context.WithTimeout(context.Background(), guard)
	defer cancelNext()
	if result := harness.run(next, "ask", "next"); result.err != nil || len(result.effects) != 1 || result.effects[0].Text != "next" {
		test.Errorf("next request: %v, %v", result.effects, result.err)
	}
	if starts := harness.starts(test); len(starts) != 2 {
		test.Errorf("%d starts, want two", len(starts))
	}
}

// Expiring before the write leaves complete framing, so the next run should
// keep using the same process rather than restarting it.
func TestServiceExpiredBeforeWriteKeepsProcess(test *testing.T) {
	harness := backpressureHarness(test)
	if result := harness.run(context.Background(), "ask", "ready"); result.err != nil {
		test.Fatal(result.err)
	}
	harness.runner.withTimeout = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
		return context.WithDeadline(parent, time.Now().Add(-time.Second))
	}
	if result := harness.run(context.Background(), "ask", "expired"); !errors.Is(result.err, ErrTimeout) {
		test.Fatalf("expired run: %v", result.err)
	}
	harness.runner.withTimeout = context.WithTimeout
	if result := harness.run(context.Background(), "ask", "next"); result.err != nil || len(result.effects) != 1 || result.effects[0].Text != "next" {
		test.Errorf("next request: %v, %v", result.effects, result.err)
	}
	if starts := harness.starts(test); len(starts) != 1 {
		test.Errorf("%d starts, want the existing process", len(starts))
	}
}

func TestServiceWaitingWriterCanCancel(test *testing.T) {
	for _, expired := range []bool{false, true} {
		test.Run(strconv.FormatBool(expired), func(test *testing.T) {
			harness := backpressureHarness(test)
			if result := harness.run(context.Background(), "ask", "block"); result.err != nil {
				test.Fatal(result.err)
			}
			serving := harness.runner.entries["ask"].service
			serving.mutex.Lock()
			process := serving.process
			serving.mutex.Unlock()
			process.writing <- struct{}{}
			defer func() { <-process.writing }()
			var ctx context.Context
			var cancel context.CancelFunc
			if expired {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			defer cancel()
			result := make(chan outcome, 1)
			go func() { result <- harness.run(ctx, "ask", "queued") }()
			if result := receive(test, result); !errors.Is(result.err, ctx.Err()) {
				test.Errorf("queued run: %v, want %v", result.err, ctx.Err())
			}
		})
	}
}

// A dead process cannot receive another run while a child holds its output.
func TestServiceRestartsBeforeOldOutputDrains(test *testing.T) {
	harness := backpressureHarness(test)
	exited := make(chan string, 8)
	harness.runner.exited = func(name string) { exited <- name }
	harness.runner.waitDelay = time.Minute
	test.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(harness.directory, "output-holder"))
		if err != nil {
			return
		}
		identifier, err := strconv.Atoi(string(data))
		if err != nil {
			test.Error(err)
			return
		}
		_ = syscall.Kill(identifier, syscall.SIGUSR1)
	})
	result := make(chan outcome, 1)
	go func() { result <- harness.run(context.Background(), "ask", "exit-with-open-output") }()
	receive(test, exited)
	if next := harness.run(context.Background(), "ask", "next"); next.err != nil || len(next.effects) != 1 || next.effects[0].Text != "next" {
		test.Errorf("next run joined dead process: %v, %v", next.effects, next.err)
	}
	data, err := os.ReadFile(filepath.Join(harness.directory, "output-holder"))
	if err != nil {
		test.Fatal(err)
	}
	identifier, err := strconv.Atoi(string(data))
	if err != nil {
		test.Fatal(err)
	}
	_ = syscall.Kill(identifier, syscall.SIGUSR1)
	if old := receive(test, result); !errors.Is(old.err, ErrExited) {
		test.Errorf("old run: %v", old.err)
	}
}

// onceLiveContext reports no error on its first check, as the write does
// before it starts, and the real one afterwards. It never reports done, so the
// waiting for the writer cannot end early. Its deadline has passed, so the
// write is interrupted without a single byte written.
type onceLiveContext struct {
	context.Context
	checked bool
}

func (ctx *onceLiveContext) Done() <-chan struct{} { return nil }

func (ctx *onceLiveContext) Err() error {
	if !ctx.checked {
		ctx.checked = true
		return nil
	}
	return ctx.Context.Err()
}

// A write that is interrupted with no byte written leaves complete framing, so
// the service stays usable.
func TestServiceInterruptedWriteWithNothingWrittenKeepsProcess(test *testing.T) {
	harness := backpressureHarness(test)
	if result := harness.run(context.Background(), "ask", "block"); result.err != nil { // it stops reading now
		test.Fatal(result.err)
	}
	serving := harness.runner.entries["ask"].service
	serving.mutex.Lock()
	process := serving.process
	serving.mutex.Unlock()
	fillPipe(test, process.input)
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	err := serving.write(&onceLiveContext{Context: expired}, process, []byte("{}\n"))
	if !errors.Is(err, context.DeadlineExceeded) {
		test.Fatalf("write to a full pipe: %v", err)
	}
	serving.mutex.Lock()
	current := serving.process
	serving.mutex.Unlock()
	if current != process {
		test.Error("an interrupted write with nothing written retired the service")
	}
}

// fillPipe writes one byte at a time until not even one more fits.
func fillPipe(test *testing.T, pipe *os.File) {
	test.Helper()
	raw, err := pipe.SyscallConn()
	if err != nil {
		test.Fatal(err)
	}
	var full error
	err = raw.Write(func(descriptor uintptr) bool {
		for {
			if _, full = syscall.Write(int(descriptor), []byte{'x'}); full != nil {
				return true
			}
		}
	})
	if err != nil {
		test.Fatal(err)
	}
	if !errors.Is(full, syscall.EAGAIN) {
		test.Fatalf("filling the pipe: %v", full)
	}
}
