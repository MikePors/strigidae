package workspace

import (
	"testing"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// autoHideHarness opens client.go with the diff focused and the cursor on its
// first hunk, with autoviewed and the given hide options on.
func autoHideHarness(test *testing.T, hideViewed, hideReviewed bool) *hideHarness {
	test.Helper()
	hidden := newHideHarness(test, false)
	hidden.send(userinterface.OptionsMessage{Values: options.Values{
		options.AutoViewed:   true,
		options.HideViewed:   hideViewed,
		options.HideReviewed: hideReviewed,
	}})
	hidden.press("j", "l")
	return hidden
}

// TestAutoViewedFileFadesAndCanBeTakenBack: an auto-marked file goes through
// the same fade as one marked with v, and v on it while it fades undoes it.
func TestAutoViewedFileFadesAndCanBeTakenBack(test *testing.T) {
	hidden := autoHideHarness(test, true, false)
	hidden.press("m", "]", "m")
	if hidden.stateOf() != viewed || hidden.serverState() != viewed {
		test.Fatalf("file is %q here and %q on the server, want viewed", hidden.stateOf(), hidden.serverState())
	}
	if !hidden.view.fades.active(fadeKey(firstFile)) {
		test.Fatal("the auto-marked file is not fading")
	}
	hidden.wantRows(0, 1, 2, 3, 4, 5) // still listed

	hidden.fire(fadeDuration)
	hidden.wantRows(0, 2, 3, 4, 5)

	// Back to a second run, taking the mark back while it fades.
	again := autoHideHarness(test, true, false)
	again.press("m", "]", "m", "h", "k", "v")
	if again.stateOf() != unviewed || again.serverState() != unviewed {
		test.Errorf("file is %q here and %q on the server, want unviewed", again.stateOf(), again.serverState())
	}
	if again.view.fades.active(fadeKey(firstFile)) {
		test.Error("the fade goes on after the undo")
	}
	again.fire(fadeDuration)
	again.wantRows(0, 1, 2, 3, 4, 5)
	if _, auto := again.view.hunkMarks.autoViewed[fileKey{again.view.repository, again.view.number, clientPath}]; auto {
		test.Error("a file the user took back is still remembered as auto-marked")
	}
}

// TestAutoViewedWithHideReviewedMarksOnce: the last hunk fading out does not
// mark the file again, and no second mutation is sent.
func TestAutoViewedWithHideReviewedMarksOnce(test *testing.T) {
	hidden := autoHideHarness(test, false, true)
	hidden.press("m", "]", "m")
	if hidden.stateOf() != viewed {
		test.Fatal("reviewing the last hunk did not mark the file viewed")
	}
	if got := viewedSent(hidden.client); len(got) != 1 || !got[0] {
		test.Fatalf("GitHub got %v, want one viewed=true", got)
	}
	hidden.fire(fadeDuration)
	hidden.fire(fadeDuration)
	if got := viewedSent(hidden.client); len(got) != 1 {
		test.Errorf("after the hunks faded GitHub got %v, want still one call", got)
	}
	if hidden.stateOf() != viewed {
		test.Error("the file stopped being viewed when the hunk faded")
	}
}
