package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestHideResolvedOption(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	harness.selectFile()
	const resolvedHeader = "▸ carol 1 comment · 1d ago [resolved]"
	if !strings.Contains(harness.screen(), resolvedHeader) {
		test.Fatalf("resolved thread should show by default:\n%s", harness.screen())
	}

	harness.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: true}})
	if out := harness.screen(); strings.Contains(out, "[resolved]") || !strings.Contains(out, "▾ bob 2 comments") {
		test.Errorf("hideresolved should leave out only the resolved thread:\n%s", out)
	}
	harness.press(")")
	if strings.Contains(harness.cursorRow(), "carol") {
		test.Errorf(") should skip the hidden thread, landed on %q", harness.cursorRow())
	}

	harness.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: false}})
	if !strings.Contains(harness.screen(), resolvedHeader) {
		test.Errorf("nohideresolved should show the resolved thread again:\n%s", harness.screen())
	}
}

// fadingInliner is an inliner with hideresolved on, a clock the test moves
// and fade timers it fires.
type fadingInliner struct {
	*inliner
	clock time.Time
	ticks []pendingTick
}

func newFadingInliner(test *testing.T) *fadingInliner {
	test.Helper()
	fading := &fadingInliner{inliner: newInliner(test), clock: testNow}
	fading.view.now = func() time.Time { return fading.clock }
	fading.view.tick = func(after time.Duration, function func(time.Time) tea.Msg) tea.Cmd {
		fading.ticks = append(fading.ticks, pendingTick{after, function})
		return nil
	}
	fading.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: true}})
	return fading
}

// fire moves the clock on by one fade interval and delivers the waiting
// timers.
func (fading *fadingInliner) fire() {
	fading.test.Helper()
	fading.clock = fading.clock.Add(fadeInterval)
	waiting := fading.ticks
	fading.ticks = nil
	for _, tick := range waiting {
		fading.send(tick.function(fading.clock))
	}
}

// fireAll fires timers until none are left.
func (fading *fadingInliner) fireAll() {
	fading.test.Helper()
	for range theme.FadeSteps + 2 {
		fading.fire()
	}
	if len(fading.ticks) != 0 {
		fading.test.Fatalf("fade still running: %d timers", len(fading.ticks))
	}
}

func (fading *fadingInliner) resolve(identifier string) {
	fading.test.Helper()
	fading.run(fading.ask(identifier).OnYes)
	if !fading.threadState(identifier).IsResolved {
		fading.test.Fatalf("setup: %s was not resolved", identifier)
	}
}

const openThreadText = "bob 2 comments"

func TestHideResolvedThreadFadesThenLeaves(test *testing.T) {
	fading := newFadingInliner(test)
	fading.resolve("t-open")
	if !strings.Contains(fading.shown(), openThreadText) {
		test.Fatalf("a thread just resolved should stay while it fades:\n%s", fading.shown())
	}
	if len(fading.ticks) != 1 || fading.ticks[0].after != fadeInterval {
		test.Fatalf("fade timers = %v, want one after %v", fading.ticks, fadeInterval)
	}
	fading.fire()
	if step, ok := fading.view.threadFade("t-open"); !ok || step != 1 {
		test.Errorf("after one interval: step %d, fading %v; want step 1, fading", step, ok)
	}
	if !strings.Contains(fading.shown(), openThreadText) {
		test.Error("the thread left before its fade was over")
	}
	fading.fireAll()
	if shown := fading.shown(); strings.Contains(shown, openThreadText) {
		test.Errorf("the thread should leave once its fade is over:\n%s", shown)
	}
	if cursor, ok := fading.diffView().Cursor(); !ok || cursor.New != 14 {
		test.Errorf("cursor = %+v (ok %v), want new line 14", cursor, ok)
	}
}

func TestHideResolvedUnresolveWhileFadingKeepsThread(test *testing.T) {
	fading := newFadingInliner(test)
	fading.resolve("t-open")
	fading.fire()
	confirmation := fading.ask("t-open")
	if !strings.HasPrefix(confirmation.Text, "Unresolve thread") {
		test.Fatalf("x on a fading thread asked %q, want to unresolve", confirmation.Text)
	}
	fading.run(confirmation.OnYes)
	if fading.threadState("t-open").IsResolved {
		test.Fatal("setup: the thread was not unresolved")
	}
	if _, ok := fading.view.threadFade("t-open"); ok {
		test.Error("unresolving should end the fade")
	}
	fading.fire() // the cancelled run's timer is ignored
	if len(fading.ticks) != 0 {
		test.Errorf("a cancelled fade asked for more timers: %d", len(fading.ticks))
	}
	if !strings.Contains(fading.shown(), openThreadText) {
		test.Errorf("an unresolved thread should stay:\n%s", fading.shown())
	}
}

func TestHideResolvedOffEndsFade(test *testing.T) {
	fading := newFadingInliner(test)
	fading.resolve("t-open")
	fading.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: false}})
	if _, ok := fading.view.threadFade("t-open"); ok {
		test.Error(":set nohideresolved should end the fade")
	}
	fading.fire()
	if !strings.Contains(fading.shown(), openThreadText) {
		test.Errorf("with the option off a resolved thread stays:\n%s", fading.shown())
	}
}

func TestHideResolvedArrivingResolvedHidesAtOnce(test *testing.T) {
	fading := newFadingInliner(test)
	if len(fading.ticks) != 0 || strings.Contains(fading.shown(), "[resolved]") {
		test.Errorf("a thread loaded resolved should hide with no fade; timers %d:\n%s", len(fading.ticks), fading.shown())
	}
}
