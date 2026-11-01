//go:build unix

package plugins

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/config"
)

// guard bounds a wait that only a broken implementation would not finish.
const guard = 20 * time.Second

func scriptPlugin(command, script string, timeout time.Duration) config.Plugin {
	return config.Plugin{
		Command: command,
		OnRun:   []string{filepath.Join("testdata", script)},
		Target:  config.TargetLines,
		Timeout: timeout,
	}
}

func baseEnvironment(directory string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "FAKE_DIR=" + directory}, extra...)
}

func receive[Value any](test *testing.T, channel <-chan Value) Value {
	test.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(guard):
		test.Fatal("timed out waiting")
		panic("unreachable")
	}
}

type outcome struct {
	effects []Effect
	err     error
}

func runInBackground(ctx context.Context, runner *Runner, name string, body string) <-chan outcome {
	results := make(chan outcome, 1)
	go func() {
		effects, err := runner.execute(ctx, runner.entries[name], []byte(body))
		results <- outcome{effects, err}
	}()
	return results
}

func TestRunEffects(test *testing.T) {
	notice := func(text string) Effect { return Effect{Name: EffectNotice, Text: text} }
	for _, testCase := range []struct {
		name      string
		output    string
		exit      int
		want      []Effect
		wantError string
		wantIs    error
		wantLog   string
	}{
		{name: "no output", want: nil},
		{name: "notice", output: `{"name":"notice","data":{"text":"hi"}}` + "\n", want: []Effect{notice("hi")}},
		{name: "answer", output: `{"name":"answer","data":{"text":"42"}}` + "\n", want: []Effect{{Name: EffectAnswer, Text: "42"}}},
		{name: "reload", output: `{"name":"reload","data":{}}` + "\n", want: []Effect{{Name: EffectReload}}},
		{name: "reload without data", output: `{"name":"reload"}` + "\n", want: []Effect{{Name: EffectReload}}},
		{name: "last line without newline", output: `{"name":"notice","data":{"text":"hi"}}`, want: []Effect{notice("hi")}},
		{
			name:   "error is an effect, not a failure",
			output: `{"name":"error","data":{"text":"nope"}}` + "\n",
			want:   []Effect{{Name: EffectError, Text: "nope"}},
		},
		{
			name: "order is kept, blank lines and carriage returns skipped",
			output: `{"name":"notice","data":{"text":"1"}}` + "\r\n\n   \n" +
				`{"name":"answer","data":{"text":"2"}}` + "\n" +
				`{"name":"reload","data":{}}` + "\n" +
				`{"name":"notice","data":{"text":"3"}}` + "\n",
			want: []Effect{notice("1"), {Name: EffectAnswer, Text: "2"}, {Name: EffectReload}, notice("3")},
		},
		{
			name: "unknown effect is skipped and logged",
			output: `{"name":"notice","data":{"text":"a"}}` + "\n" +
				`{"name":"future","data":{"x":1}}` + "\n" +
				`{"name":"notice","data":{"text":"b"}}` + "\n",
			want:    []Effect{notice("a"), notice("b")},
			wantLog: `skipping unknown effect "future"`,
		},
		{name: "not JSON", output: "hello\n", wantIs: ErrInvalidOutput, wantError: `plugin "p": line 1: plugin printed a line that is not an effect`},
		{
			name:      "bad line after a good one",
			output:    `{"name":"notice","data":{"text":"a"}}` + "\nhello\n",
			wantIs:    ErrInvalidOutput,
			wantError: `plugin "p": line 2: plugin printed a line that is not an effect`,
		},
		{name: "not an object", output: "[1]\n", wantIs: ErrInvalidOutput, wantError: `plugin "p": line 1: plugin printed a line that is not an effect`},
		{name: "no name", output: `{"data":{}}` + "\n", wantIs: ErrInvalidOutput, wantError: `plugin "p": line 1: plugin printed a line that is not an effect`},
		{name: "text effect with data that is not an object", output: `{"name":"notice","data":"hi"}` + "\n", wantIs: ErrInvalidOutput, wantError: `plugin "p": line 1: plugin printed a line that is not an effect`},
		{name: "notice without data", output: `{"name":"notice"}` + "\n", wantIs: ErrInvalidOutput, wantError: `plugin "p": line 1: plugin printed a line that is not an effect`},
		{name: "failure without an error effect", exit: 2, wantIs: ErrFailed, wantError: `plugin "p": plugin failed`},
		{
			name:      "failure with other effects only",
			output:    `{"name":"notice","data":{"text":"a"}}` + "\n",
			exit:      1,
			wantIs:    ErrFailed,
			wantError: `plugin "p": plugin failed`,
		},
		{
			name:      "failure reports the error effect's text",
			output:    `{"name":"notice","data":{"text":"a"}}` + "\n" + `{"name":"error","data":{"text":"no model"}}` + "\n",
			exit:      1,
			wantIs:    ErrFailed,
			wantError: `plugin "p": no model`,
		},
		{name: "failure with unreadable output", output: "garbage\n", exit: 1, wantIs: ErrFailed, wantError: `plugin "p": plugin failed`},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			var log bytes.Buffer
			runner := New(
				[]config.Plugin{scriptPlugin("p", "emit.sh", guard)},
				Options{
					Environment: baseEnvironment(test.TempDir(), "FAKE_OUTPUT="+testCase.output, "FAKE_EXIT="+strconv.Itoa(testCase.exit)),
					Log:         &log,
				},
			)
			got, err := runner.Run(context.Background(), "p", Request{})
			if testCase.wantError == "" {
				if err != nil {
					test.Fatalf("Run: %v", err)
				}
			} else {
				if err == nil || err.Error() != testCase.wantError {
					test.Fatalf("error = %v, want %q", err, testCase.wantError)
				}
				if !errors.Is(err, testCase.wantIs) {
					test.Errorf("error %v is not %v", err, testCase.wantIs)
				}
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("effects mismatch (-want +got):\n%s", diff)
			}
			if !strings.Contains(log.String(), testCase.wantLog) {
				test.Errorf("log = %q, want it to contain %q", log.String(), testCase.wantLog)
			}
		})
	}
}

func TestRunWritesProtocolRequest(test *testing.T) {
	directory := test.TempDir()
	runner := New([]config.Plugin{scriptPlugin("p", "save-input.sh", guard)}, Options{Environment: baseEnvironment(directory)})
	// The caller cannot change the version.
	if _, err := runner.Run(context.Background(), "p", Request{Protocol: 99}); err != nil {
		test.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(directory, "input"))
	if err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff(`{"protocol":1}`, string(got)); diff != "" {
		test.Errorf("stdin mismatch (-want +got):\n%s", diff)
	}
}

func TestRunUnknownPlugin(test *testing.T) {
	runner := New(nil, Options{})
	_, err := runner.Run(context.Background(), "nothing", Request{})
	if !errors.Is(err, ErrUnknownPlugin) {
		test.Errorf("error = %v, want ErrUnknownPlugin", err)
	}
}

func TestRunStartFailure(test *testing.T) {
	runner := New([]config.Plugin{{Command: "p", OnRun: []string{"testdata/does-not-exist"}, Timeout: guard}}, Options{})
	_, err := runner.Run(context.Background(), "p", Request{})
	if err == nil || !strings.Contains(err.Error(), `start plugin "p"`) {
		test.Errorf("error = %v, want a start failure", err)
	}
	if errors.Is(err, ErrFailed) {
		test.Errorf("a plugin that never started did not fail by exit status: %v", err)
	}
}

func TestRunStderrGoesToTheLogOnly(test *testing.T) {
	var log bytes.Buffer
	runner := New([]config.Plugin{scriptPlugin("p", "noisy-failure.sh", guard)}, Options{Log: &log})
	_, err := runner.Run(context.Background(), "p", Request{})
	if !errors.Is(err, ErrFailed) {
		test.Fatalf("error = %v, want ErrFailed", err)
	}
	if !strings.Contains(log.String(), "details only for the log") {
		test.Errorf("log = %q, want the plugin's standard error", log.String())
	}
	if strings.Contains(err.Error(), "details only") {
		test.Errorf("error %q carries standard error", err)
	}
	if strings.Contains(log.String(), "protocol") {
		test.Errorf("log %q contains the request", log.String())
	}
}

func TestRunRemovesTokenVariables(test *testing.T) {
	runner := New([]config.Plugin{scriptPlugin("p", "show-token.sh", guard)}, Options{
		Environment: baseEnvironment(test.TempDir(), "GH_TOKEN=a", "GITHUB_TOKEN=b", "GH_ENTERPRISE_TOKEN=c", "GITHUB_ENTERPRISE_TOKEN=d", "KEPT=yes"),
	})
	got, err := runner.Run(context.Background(), "p", Request{})
	if err != nil {
		test.Fatal(err)
	}
	want := []Effect{{Name: EffectNotice, Text: "unset|unset|unset|unset|yes"}}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("effects mismatch (-want +got):\n%s", diff)
	}
}

func TestRunTimeout(test *testing.T) {
	runner := New([]config.Plugin{scriptPlugin("p", "hang.sh", 200*time.Millisecond)}, Options{})
	_, err := runner.Run(context.Background(), "p", Request{})
	if !errors.Is(err, ErrTimeout) {
		test.Errorf("error = %v, want ErrTimeout", err)
	}
}

func TestRunOutputLimit(test *testing.T) {
	runner := New([]config.Plugin{scriptPlugin("p", "flood.sh", guard)}, Options{})
	got, err := runner.Run(context.Background(), "p", Request{})
	if !errors.Is(err, ErrOutputTooLarge) {
		test.Errorf("error = %v, want ErrOutputTooLarge", err)
	}
	if got != nil {
		test.Errorf("effects = %v, want none from a failed run", got)
	}
}

func TestLimitedBufferStopsAtTheLimit(test *testing.T) {
	stopped := false
	buffer := &limitedBuffer{limit: 5, stop: func() { stopped = true }}
	if _, err := buffer.Write([]byte("abcde")); err != nil || stopped {
		test.Fatalf("write at the limit: err = %v, stopped = %v", err, stopped)
	}
	if _, err := buffer.Write([]byte("f")); err == nil || !stopped || !buffer.exceeded() {
		test.Errorf("write past the limit: err = %v, stopped = %v, exceeded = %v", err, stopped, buffer.exceeded())
	}
	if diff := cmp.Diff("abcde", string(buffer.bytes())); diff != "" {
		test.Errorf("kept bytes mismatch (-want +got):\n%s", diff)
	}
}

// killChild stops the child a script left behind, so no test leaves a process.
func killChild(test *testing.T, directory string) {
	test.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(directory, "pid"))
		if err != nil {
			return
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
}

func TestRunChildHoldingStandardOutput(test *testing.T) {
	for _, testCase := range []struct {
		name   string
		script string
		want   []Effect
		wantIs error
	}{
		{"plugin exits 0: its effects are complete", "leave-child.sh", []Effect{{Name: EffectNotice, Text: "done"}}, nil},
		{"plugin exits 1: still a failure", "leave-child-failing.sh", nil, ErrFailed},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			directory := test.TempDir()
			killChild(test, directory)
			runner := New(
				[]config.Plugin{scriptPlugin("p", testCase.script, guard)},
				Options{Environment: baseEnvironment(directory), WaitDelay: 100 * time.Millisecond},
			)
			results := runInBackground(context.Background(), runner, "p", "{}")
			result := receive(test, results)
			if testCase.wantIs == nil && result.err != nil {
				test.Fatalf("Run: %v", result.err)
			}
			if testCase.wantIs != nil && !errors.Is(result.err, testCase.wantIs) {
				test.Fatalf("error = %v, want %v", result.err, testCase.wantIs)
			}
			if diff := cmp.Diff(testCase.want, result.effects); diff != "" {
				test.Errorf("effects mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRunPluginThatIgnoresItsInput(test *testing.T) {
	runner := New([]config.Plugin{scriptPlugin("p", "ignore-input.sh", guard)}, Options{})
	// Larger than a pipe buffer, so the write cannot finish before the exit.
	body := strings.Repeat("x", 4<<20)
	result := receive(test, runInBackground(context.Background(), runner, "p", body))
	if result.err != nil || result.effects != nil {
		test.Errorf("result = %+v, want a plain success", result)
	}
}

// queueHarness runs gated.sh, which holds its plugin until the test creates
// the gate file.
type queueHarness struct {
	runner    *Runner
	directory string
	started   chan string
	queued    chan string
}

func newQueueHarness(test *testing.T, names ...string) *queueHarness {
	test.Helper()
	directory := test.TempDir()
	fifo := filepath.Join(directory, "started")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		test.Fatal(err)
	}
	// Read-write, so opening never blocks and the script's writes never do.
	file, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { _ = file.Close() })
	harness := &queueHarness{directory: directory, started: make(chan string, 64), queued: make(chan string, 64)}
	go func() {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			harness.started <- scanner.Text()
		}
	}()

	var plugins []config.Plugin
	for _, name := range names {
		plugins = append(plugins, scriptPlugin(name, "gated.sh", guard))
	}
	harness.runner = New(plugins, Options{Environment: baseEnvironment(directory)})
	harness.runner.queued = func(name string) { harness.queued <- name }
	return harness
}

func (harness *queueHarness) openGate(test *testing.T) {
	test.Helper()
	if err := os.WriteFile(filepath.Join(harness.directory, "gate"), nil, 0o600); err != nil {
		test.Fatal(err)
	}
}

func (harness *queueHarness) read(test *testing.T, name string) string {
	test.Helper()
	data, err := os.ReadFile(filepath.Join(harness.directory, name))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		test.Fatal(err)
	}
	return string(data)
}

func TestQueueRunsOneAtATimeInOrder(test *testing.T) {
	harness := newQueueHarness(test, "p")
	results := map[string]<-chan outcome{}
	results["a"] = runInBackground(context.Background(), harness.runner, "p", "a")
	receive(test, harness.started)
	for _, label := range []string{"b", "c", "d"} {
		results[label] = runInBackground(context.Background(), harness.runner, "p", label)
		receive(test, harness.queued)
	}
	harness.openGate(test)
	for label, channel := range results {
		if result := receive(test, channel); result.err != nil {
			test.Errorf("run %s: %v", label, result.err)
		}
	}
	if diff := cmp.Diff("a\nb\nc\nd\n", harness.read(test, "order")); diff != "" {
		test.Errorf("start order mismatch (-want +got):\n%s", diff)
	}
	if overlap := harness.read(test, "overlap"); overlap != "" {
		test.Errorf("runs overlapped: %q", overlap)
	}
}

func TestQueueCancelledRunLeavesWithoutRunning(test *testing.T) {
	harness := newQueueHarness(test, "p")
	first := runInBackground(context.Background(), harness.runner, "p", "a")
	receive(test, harness.started)

	cancelledContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelled := runInBackground(cancelledContext, harness.runner, "p", "b")
	receive(test, harness.queued)
	after := runInBackground(context.Background(), harness.runner, "p", "c")
	receive(test, harness.queued)

	cancel()
	// The first run still holds the plugin, so only leaving the queue can
	// end this one.
	if result := receive(test, cancelled); !errors.Is(result.err, context.Canceled) {
		test.Errorf("cancelled run error = %v, want context.Canceled", result.err)
	}

	harness.openGate(test)
	for _, channel := range []<-chan outcome{first, after} {
		if result := receive(test, channel); result.err != nil {
			test.Errorf("run: %v", result.err)
		}
	}
	if diff := cmp.Diff("a\nc\n", harness.read(test, "order")); diff != "" {
		test.Errorf("runs mismatch (-want +got):\n%s", diff)
	}
}

func TestQueueAlreadyCancelledRunDoesNotRun(test *testing.T) {
	harness := newQueueHarness(test, "p")
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	result := receive(test, runInBackground(cancelledContext, harness.runner, "p", "a"))
	if !errors.Is(result.err, context.Canceled) {
		test.Errorf("error = %v, want context.Canceled", result.err)
	}
	if order := harness.read(test, "order"); order != "" {
		test.Errorf("the plugin ran: %q", order)
	}
}

func TestQueueCancellingARunningPlugin(test *testing.T) {
	harness := newQueueHarness(test, "p")
	runContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	running := runInBackground(runContext, harness.runner, "p", "a")
	receive(test, harness.started)
	cancel()
	if result := receive(test, running); !errors.Is(result.err, context.Canceled) {
		test.Errorf("error = %v, want context.Canceled", result.err)
	}
}

func TestQueueDifferentPluginsAreIndependent(test *testing.T) {
	harness := newQueueHarness(test, "slow", "other")
	slow := runInBackground(context.Background(), harness.runner, "slow", "a")
	receive(test, harness.started)

	// "slow" holds its turn at the gate. "other" has its own queue, so it starts
	// at once; the shared lock directory makes it record an overlap here, which
	// is expected.
	other := runInBackground(context.Background(), harness.runner, "other", "b")
	receive(test, harness.started)
	harness.openGate(test)
	for _, channel := range []<-chan outcome{slow, other} {
		if result := receive(test, channel); result.err != nil {
			test.Errorf("run: %v", result.err)
		}
	}
	select {
	case name := <-harness.queued:
		test.Errorf("a run of %q queued behind another plugin", name)
	default:
	}
}

func TestQueueTimeoutStartsWithTheRun(test *testing.T) {
	harness := newQueueHarness(test, "p")
	gate := filepath.Join(harness.directory, "gate")
	gateOpen := make(chan bool, 8)
	harness.runner.withTimeout = func(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
		_, err := os.Stat(gate)
		gateOpen <- err == nil
		return context.WithTimeout(ctx, duration)
	}

	first := runInBackground(context.Background(), harness.runner, "p", "a")
	receive(test, harness.started)
	second := runInBackground(context.Background(), harness.runner, "p", "b")
	receive(test, harness.queued)
	harness.openGate(test)
	for _, channel := range []<-chan outcome{first, second} {
		if result := receive(test, channel); result.err != nil {
			test.Errorf("run: %v", result.err)
		}
	}
	// The first run's timeout started before the gate opened; the second
	// run's only after it, as the second run waited for the first.
	if firstStarted, secondStarted := receive(test, gateOpen), receive(test, gateOpen); firstStarted || !secondStarted {
		test.Errorf("gate open when the timeouts started: first %v, second %v; want false, true", firstStarted, secondStarted)
	}
}
