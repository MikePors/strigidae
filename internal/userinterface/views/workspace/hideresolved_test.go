package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// resolveOpenThread resolves t-open with x and y.
func (fading *fadingInliner) resolveOpenThread() {
	fading.test.Helper()
	fading.run(fading.ask("t-open").OnYes)
	if !fading.threadState("t-open").IsResolved {
		fading.test.Fatal("setup: t-open was not resolved")
	}
}

const openThreadText = "bob 2 comments"

func TestHideResolvedThreadFadesThenLeaves(test *testing.T) {
	fading := newFadingInliner(test)
	fading.resolveOpenThread()
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
	fading.resolveOpenThread()
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
	fading.resolveOpenThread()
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
	if strings.Contains(fading.shown(), "[resolved]") {
		test.Errorf("a thread loaded resolved should hide at once:\n%s", fading.shown())
	}
}

// TestHideResolvedFadesWhileThreadsReload: a resolve that lands while a
// thread load is in flight is not applied in place, so the screen holds the
// thread unresolved until the reload arrives; the fade must still run.
func TestHideResolvedFadesWhileThreadsReload(test *testing.T) {
	fading := newFadingInliner(test)
	_, write := fading.view.Update(fading.ask("t-open").OnYes())
	fading.view.threadPending = true // a refresh is in flight
	_, reload := fading.view.Update(write())
	fading.fire()
	if _, ok := fading.view.threadFade("t-open"); !ok {
		test.Fatal("the fade stopped while the reload was in flight")
	}
	fading.run(reload)
	if !fading.threadState("t-open").IsResolved {
		test.Fatal("setup: the reload did not bring the thread resolved")
	}
	if !strings.Contains(fading.shown(), openThreadText) {
		test.Errorf("the thread should stay until its fade is over:\n%s", fading.shown())
	}
	fading.fireAll()
	if strings.Contains(fading.shown(), openThreadText) {
		test.Errorf("the thread should leave once its fade is over:\n%s", fading.shown())
	}
}

func TestHideResolvedUnresolvedByOthersStopsFading(test *testing.T) {
	fading := newFadingInliner(test)
	fading.resolveOpenThread()
	fading.fire()
	if _, err := fading.client.UnresolveReviewThread(test.Context(), "t-open"); err != nil {
		test.Fatal(err)
	}
	fading.run(fading.view.reloadThreads())
	if _, ok := fading.view.threadFade("t-open"); ok {
		test.Error("a thread a reload shows unresolved should stop fading at once")
	}
	fading.fireAll()
	if !strings.Contains(fading.shown(), openThreadText) {
		test.Errorf("a thread unresolved by someone else should stay:\n%s", fading.shown())
	}
}

// TestHideResolvedThreadIsDrawnFaded checks the drawing itself, with the
// workspace's hunk fades installed as in the app.
func TestHideResolvedThreadIsDrawnFaded(test *testing.T) {
	fading := newFadingInliner(test)
	fading.resolveOpenThread()
	fading.fire()
	width, height := fading.diffView().Size()
	for _, line := range strings.Split(fading.diffView().Render(width, height), "\n") {
		if strings.Contains(ansi.Strip(line), openThreadText) {
			if want := fading.view.theme.Fade[0].Render(ansi.Strip(line)); line != want {
				test.Errorf("thread header drawn %q, want it in the first fade step %q", line, want)
			}
			return
		}
	}
	test.Fatal("the fading thread is not drawn")
}
