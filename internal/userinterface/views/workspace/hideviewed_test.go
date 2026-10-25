package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// hideHarness is a workspace with the hideviewed option on, a clock the test
// moves, and timers that wait for the test to fire them.
type hideHarness struct {
	*harness
	clock time.Time
	ticks []pendingTick
}

type pendingTick struct {
	after    time.Duration
	function func(time.Time) tea.Msg
}

const (
	viewed   = github.FileViewed
	unviewed = github.FileUnviewed
)

func newHideHarness(test *testing.T, hide bool, states ...github.ViewedState) *hideHarness {
	test.Helper()
	hidden := &hideHarness{harness: viewedHarness(test, 100, 30, states...), clock: testNow}
	hidden.view.now = func() time.Time { return hidden.clock }
	hidden.view.tick = func(after time.Duration, function func(time.Time) tea.Msg) tea.Cmd {
		hidden.ticks = append(hidden.ticks, pendingTick{after, function})
		return nil
	}
	if hide {
		hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideViewed: true}})
	}
	return hidden
}

// fire moves the clock forward and delivers the timers waiting now.
func (hidden *hideHarness) fire(elapsed time.Duration) {
	hidden.test.Helper()
	hidden.clock = hidden.clock.Add(elapsed)
	waiting := hidden.ticks
	hidden.ticks = nil
	for _, tick := range waiting {
		hidden.send(tick.function(hidden.clock))
	}
}

func (hidden *hideHarness) wantRows(want ...int) {
	hidden.test.Helper()
	if diff := cmp.Diff(want, hidden.view.rows); diff != "" {
		hidden.test.Errorf("Files pane rows (-want +got):\n%s", diff)
	}
}

func TestHideViewedLeavesViewedFilesOutAndBringsThemBack(test *testing.T) {
	hidden := newHideHarness(test, false, viewed, unviewed, viewed)
	hidden.wantRows(0, 1, 2, 3, 4, 5)

	hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideViewed: true}})
	hidden.wantRows(0, 2, 4, 5)
	screen := hidden.screen()
	if strings.Contains(screen, "sync/client.go") || !strings.Contains(screen, "retry.go") || !strings.Contains(screen, "Overview") {
		test.Errorf("hidden screen:\n%s", screen)
	}

	hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideViewed: false}})
	hidden.wantRows(0, 1, 2, 3, 4, 5)
	if !strings.Contains(hidden.screen(), "sync/client.go") {
		test.Errorf("nohideviewed did not bring the file back:\n%s", hidden.screen())
	}
}

func TestHideViewedTitleStillCountsEveryFile(test *testing.T) {
	hidden := newHideHarness(test, true, viewed, unviewed, viewed)
	if got := hidden.view.filesTitle(); got != "Files 2/5 viewed" {
		test.Errorf("title = %q", got)
	}
}

func TestViewedMovesTheCursorOnAndTheFileLeavesAfterTheGracePeriod(test *testing.T) {
	hidden := newHideHarness(test, true)
	hidden.press("j", "v") // client.go
	if got := hidden.view.selection(); got != 2 {
		test.Errorf("after v the cursor is on entry %d, want 2", got)
	}
	hidden.wantRows(0, 1, 2, 3, 4, 5) // still listed
	if len(hidden.ticks) != 1 || hidden.ticks[0].after != fadeInterval {
		test.Fatalf("timers = %+v, want one of %v", hidden.ticks, fadeInterval)
	}
	if hidden.stateOf() != viewed {
		test.Error("v did not mark the file at once")
	}

	for step := 1; step <= theme.FadeSteps; step++ {
		hidden.fire(fadeInterval)
		if got := hidden.view.fades.step(fadeKey(firstFile)); got != step {
			test.Fatalf("fade step = %d, want %d", got, step)
		}
		hidden.wantRows(0, 1, 2, 3, 4, 5)
		if len(hidden.ticks) != 1 {
			test.Fatalf("step %d: %d timers waiting, want 1", step, len(hidden.ticks))
		}
	}
	hidden.fire(fadeInterval) // 8 seconds in
	hidden.wantRows(0, 2, 3, 4, 5)
	if len(hidden.ticks) != 0 {
		test.Errorf("%d timers left after the file left", len(hidden.ticks))
	}
	if got := hidden.view.selection(); got != 2 {
		test.Errorf("the cursor moved to entry %d when the file left, want it to stay on 2", got)
	}
	if hidden.serverState() != viewed {
		test.Error("the file is not viewed on the server")
	}
}

func TestFadingRowsUseTheThemesFadeStyles(test *testing.T) {
	hidden := newHideHarness(test, true)
	hidden.press("j", "v")
	for step := 1; step <= theme.FadeSteps; step++ {
		hidden.fire(fadeInterval)
		opening, _, _ := strings.Cut(hidden.view.theme.Fade[step-1].Render("x"), "x")
		if opening == "" {
			test.Fatalf("fade style %d adds no styling", step)
		}
		row := hidden.view.fileRows(40, 10)[1]
		if !strings.Contains(row, opening) || !strings.Contains(row, "client.go") {
			test.Errorf("step %d row %q lacks the theme's fade style %q", step, row, opening)
		}
	}
}

func TestViewedAtTheEndMovesTheCursorBack(test *testing.T) {
	hidden := newHideHarness(test, true, viewed, viewed, viewed, viewed)
	hidden.wantRows(0, 5)
	hidden.press("j", "v")
	if got := hidden.view.selection(); got != 0 {
		test.Errorf("after the last file the cursor is on entry %d, want Overview", got)
	}

	other := newHideHarness(test, true, viewed)
	other.press("G", "v")
	if got := other.view.selection(); got != 4 {
		test.Errorf("after the last file the cursor is on entry %d, want the one before (4)", got)
	}
}

func TestViewedAgainWhileFadingTakesTheMarkBack(test *testing.T) {
	hidden := newHideHarness(test, true)
	hidden.press("j", "v", "k", "v")
	if hidden.stateOf() != unviewed || hidden.serverState() != unviewed {
		test.Errorf("file is %q here and %q on the server, want unviewed", hidden.stateOf(), hidden.serverState())
	}
	if hidden.view.fades.active(fadeKey(firstFile)) {
		test.Error("the fade goes on")
	}
	hidden.fire(fadeDuration)
	hidden.wantRows(0, 1, 2, 3, 4, 5)
}

func TestStaleFadeTimersAreIgnored(test *testing.T) {
	test.Run("after the option is turned off", func(test *testing.T) {
		hidden := newHideHarness(test, true)
		hidden.press("j", "v")
		hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideViewed: false}})
		hidden.fire(fadeDuration)
		hidden.wantRows(0, 1, 2, 3, 4, 5)
		if hidden.stateOf() != viewed {
			test.Error("turning the option off un-marked the file")
		}
		hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideViewed: true}})
		hidden.wantRows(0, 2, 3, 4, 5) // viewed and no longer fading
	})
	test.Run("after the mark was taken back and given again", func(test *testing.T) {
		hidden := newHideHarness(test, true)
		hidden.press("j", "v", "k", "v", "v")
		first := hidden.ticks[0]
		hidden.ticks = hidden.ticks[1:]
		hidden.clock = hidden.clock.Add(fadeDuration)
		hidden.send(first.function(hidden.clock)) // the first run's timer
		hidden.wantRows(0, 1, 2, 3, 4, 5)
		hidden.fire(0) // the second run's timer, now due
		hidden.wantRows(0, 2, 3, 4, 5)
	})
	test.Run("from another workspace", func(test *testing.T) {
		hidden := newHideHarness(test, true)
		hidden.press("j", "v")
		hidden.clock = hidden.clock.Add(fadeDuration)
		hidden.send(fadeMessage{owner: nil, key: fadeKey(firstFile), generation: 1})
		hidden.wantRows(0, 1, 2, 3, 4, 5)
	})
}

func TestNextAndPreviousFileSkipHiddenFiles(test *testing.T) {
	hidden := newHideHarness(test, true, unviewed, viewed, viewed)
	hidden.press("j", "}")
	if got := hidden.view.selection(); got != 4 {
		test.Errorf("} went to entry %d, want 4", got)
	}
	hidden.press("{")
	if got := hidden.view.selection(); got != 1 {
		test.Errorf("{ went to entry %d, want 1", got)
	}
}

func TestPickerListsHiddenFilesAndKeepsTheCursorOnAShownOne(test *testing.T) {
	hidden := newHideHarness(test, true, unviewed, unviewed, viewed)
	hidden.press("f")
	if got := hidden.view.picker.Matches(); got != 5 {
		test.Errorf("the picker lists %d files, want all 5", got)
	}
	hidden.typeText("legacy")
	hidden.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := hidden.view.selectedPath(); got != "internal/sync/legacy.go" {
		test.Errorf("opened %q", got)
	}
	if got := hidden.view.cursorEntry(); got != 4 {
		test.Errorf("the Files pane cursor is on entry %d, want the nearest shown file (4)", got)
	}
	hidden.press("j")
	if got := hidden.view.selection(); got != 5 {
		test.Errorf("j from the cursor went to entry %d, want 5", got)
	}
}
