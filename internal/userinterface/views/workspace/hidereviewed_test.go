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

const (
	firstPlaceholder  = "▸ reviewed hunk hidden  " + firstHunkHeader
	secondPlaceholder = "▸ reviewed hunk hidden  " + secondHunkHeader
	firstHunkLine     = "return Retry(ctx, 5"
)

// newHideReviewedHarness opens client.go with the diff focused, like
// hunkHarness, with a clock the test moves and timers it fires.
func newHideReviewedHarness(test *testing.T, hide bool) *hideHarness {
	test.Helper()
	hidden := &hideHarness{harness: hunkHarness(test), clock: testNow}
	hidden.view.now = func() time.Time { return hidden.clock }
	hidden.view.tick = func(after time.Duration, function func(time.Time) tea.Msg) tea.Cmd {
		hidden.ticks = append(hidden.ticks, pendingTick{after, function})
		return nil
	}
	hidden.setHideReviewed(hide)
	return hidden
}

func (hidden *hideHarness) setHideReviewed(hide bool) {
	hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideReviewed: hide}})
}

// shows reports whether the screen holds text.
func (hidden *hideHarness) shows(text string) bool { return strings.Contains(hidden.screen(), text) }

func TestHideReviewedFadesThenLeavesPlaceholder(test *testing.T) {
	hidden := newHideReviewedHarness(test, true)
	hidden.press("m")
	if got := hidden.lastInfo(); got != "hunk 1/2 reviewed" {
		test.Errorf("status = %q", got)
	}
	if len(hidden.ticks) != 1 || hidden.ticks[0].after != fadeInterval {
		test.Fatalf("timers = %+v, want one of %v", hidden.ticks, fadeInterval)
	}
	for step := 1; step <= theme.FadeSteps; step++ {
		hidden.fire(fadeInterval)
		file, _, _ := hidden.view.filePane.CursorHunk()
		if got := hidden.view.hunkFade(file, 0); got != step {
			test.Fatalf("fade step = %d, want %d", got, step)
		}
		if !hidden.shows(firstHunkLine) || hidden.shows(firstPlaceholder) {
			test.Fatalf("step %d: the hunk left before the grace period ended:\n%s", step, hidden.screen())
		}
	}
	hidden.fire(fadeInterval) // 8 seconds in
	screen := hidden.screen()
	if strings.Contains(screen, firstHunkLine) || !strings.Contains(screen, ">"+firstPlaceholder) {
		test.Errorf("after the grace period, want the placeholder under the cursor:\n%s", screen)
	}
	if !strings.Contains(screen, secondHunkHeader) || strings.Contains(screen, secondPlaceholder) {
		test.Errorf("the second hunk is not shown:\n%s", screen)
	}
	if len(hidden.ticks) != 0 {
		test.Errorf("%d timers left after the hunk left", len(hidden.ticks))
	}
}

func TestHideReviewedUndoWhileFading(test *testing.T) {
	hidden := newHideReviewedHarness(test, true)
	hidden.press("m")
	hidden.fire(fadeInterval)
	hidden.press("m")
	if got := hidden.lastInfo(); got != "hunk 1/2 not reviewed" {
		test.Errorf("status = %q", got)
	}
	file, _, _ := hidden.view.filePane.CursorHunk()
	if hidden.view.fades.active(hidden.view.hunkFadeKey(file, 0)) {
		test.Error("the hunk is still fading after it was unmarked")
	}
	hidden.fire(fadeDuration) // the late timer
	if !hidden.shows(firstHunkLine) || hidden.shows(firstPlaceholder) {
		test.Errorf("the unmarked hunk left:\n%s", hidden.screen())
	}
}

func TestHideReviewedPlaceholderUnmarks(test *testing.T) {
	hidden := newHideReviewedHarness(test, true)
	hidden.press("m")
	hidden.fire(fadeDuration)
	hidden.fire(fadeDuration)
	if !hidden.shows(">" + firstPlaceholder) {
		test.Fatalf("setup: no placeholder under the cursor:\n%s", hidden.screen())
	}
	hidden.press("enter")
	if !hidden.shows(">" + firstPlaceholder) {
		test.Errorf("enter on the placeholder changed something:\n%s", hidden.screen())
	}
	hidden.press("m")
	if got := hidden.reviewedHeaders(); len(got) != 0 {
		test.Errorf("reviewed headers = %q, want none", got)
	}
	if !hidden.shows(">"+firstHunkHeader) || !hidden.shows(firstHunkLine) {
		test.Errorf("the unmarked hunk did not come back under the cursor:\n%s", hidden.screen())
	}
}

func TestHideReviewedNavigationSkipsPlaceholder(test *testing.T) {
	hidden := newHideReviewedHarness(test, false)
	hidden.press("m")
	hidden.setHideReviewed(true)
	hidden.press("G", "[")
	if !hidden.shows(">" + secondHunkHeader) {
		test.Errorf("[ from the bottom: want the second hunk's header, the first is hidden:\n%s", hidden.screen())
	}
	hidden.press("[")
	if !hidden.shows(">" + secondHunkHeader) {
		test.Errorf("[ moved onto the hidden hunk:\n%s", hidden.screen())
	}
	hidden.pressCounted("G", 15)
	if !hidden.shows(">" + firstPlaceholder) {
		test.Errorf("15G, a line of the hidden hunk: want its placeholder:\n%s", hidden.screen())
	}
}

// TestHideReviewedOptionTakesEffectAtOnce: hunks reviewed before the option
// is on hide without a fade; turning it off brings them back and ends fades.
func TestHideReviewedOptionTakesEffectAtOnce(test *testing.T) {
	hidden := newHideReviewedHarness(test, false)
	hidden.press("m", "]", "m")
	if len(hidden.ticks) != 0 {
		test.Errorf("marking with the option off started %d timers", len(hidden.ticks))
	}
	hidden.setHideReviewed(true)
	if !hidden.shows(firstPlaceholder) || !hidden.shows(secondPlaceholder) {
		test.Errorf("turning the option on did not hide the reviewed hunks:\n%s", hidden.screen())
	}
	hidden.setHideReviewed(false)
	if hidden.shows("▸") || len(hidden.reviewedHeaders()) != 2 {
		test.Errorf("turning it off: want both hunks back and still reviewed:\n%s", hidden.screen())
	}

	hidden.press("m") // unmark the second, then mark it again with the option on
	hidden.setHideReviewed(true)
	hidden.press("m")
	hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideReviewed: true, options.HideViewed: false}})
	if hidden.shows(secondPlaceholder) {
		test.Error("a hideviewed change ended a hunk's fade")
	}
	hidden.setHideReviewed(false)
	hidden.setHideReviewed(true)
	if !hidden.shows(secondPlaceholder) {
		test.Errorf("nohidereviewed did not end the fade: the hunk is not hidden at once:\n%s", hidden.screen())
	}
}

// TestHideReviewedFadeSurvivesRefresh: the fade follows the hunk's content
// through a refresh.
func TestHideReviewedFadeSurvivesRefresh(test *testing.T) {
	hidden := newHideReviewedHarness(test, true)
	hidden.press("m")
	hidden.press("R")
	if !hidden.shows(firstHunkLine) {
		test.Fatalf("the refresh hid the fading hunk:\n%s", hidden.screen())
	}
	hidden.fire(fadeDuration)
	hidden.fire(fadeDuration)
	if !hidden.shows(firstPlaceholder) {
		test.Errorf("the fade stopped at the refresh:\n%s", hidden.screen())
	}
}

// TestHideReviewedInReopenedWorkspace: a workspace sharing the marks hides
// the reviewed hunks at once; the first one's fade does not carry over.
func TestHideReviewedInReopenedWorkspace(test *testing.T) {
	first := newHideReviewedHarness(test, true)
	first.press("m")

	reopened := newHideReviewedHarness(test, false)
	WithHunkMarks(first.view.hunkMarks)(reopened.view)
	reopened.setHideReviewed(true)
	if !reopened.shows(firstPlaceholder) {
		test.Errorf("the reopened workspace shows the reviewed hunk:\n%s", reopened.screen())
	}
}
