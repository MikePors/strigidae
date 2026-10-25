package workspace

import (
	"errors"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// autoHarness opens client.go (two hunks in testdata/pull_request.diff) with the diff focused, cursor on its first
// hunk, and the autoviewed option set to on.
func autoHarness(test *testing.T, on bool, states ...github.ViewedState) *harness {
	test.Helper()
	harness := viewedHarness(test, 100, 30, states...)
	harness.send(userinterface.OptionsMessage{Values: options.Values{}.With(options.AutoViewed, on)})
	harness.press("j", "l")
	return harness
}

// viewedSent returns the viewed flags sent to GitHub for client.go, in order.
func viewedSent(client *fake.Client) []bool {
	var sent []bool
	for _, call := range viewedCalls(client) {
		if call.Path == clientPath {
			sent = append(sent, call.Viewed)
		}
	}
	return sent
}

func (harness *harness) clientFile() github.ViewedState { return harness.view.detail.Files[0].Viewed }

func TestAutoViewedOffByDefault(test *testing.T) {
	harness := autoHarness(test, false)
	harness.press("m", "]", "m")
	if harness.clientFile() == github.FileViewed || len(viewedSent(harness.client)) != 0 {
		test.Error("reviewing every hunk marked the file viewed with autoviewed off")
	}
}

func TestAutoViewedMarksAndUnmarks(test *testing.T) {
	harness := autoHarness(test, true)
	harness.press("m")
	if harness.clientFile() == github.FileViewed {
		test.Fatal("the file was marked viewed with a hunk still unreviewed")
	}

	harness.press("]", "m")
	if harness.clientFile() != github.FileViewed {
		test.Error("reviewing the last hunk did not mark the file viewed")
	}
	if got := harness.lastInfo(); got != "Marked "+clientPath+" viewed (all hunks reviewed)" {
		test.Errorf("status = %q", got)
	}
	if got := viewedSent(harness.client); len(got) != 1 || !got[0] {
		test.Errorf("GitHub got %v, want one viewed=true", got)
	}

	harness.press("m") // unreview the second hunk again
	if harness.clientFile() == github.FileViewed {
		test.Error("unreviewing a hunk left the auto-marked file viewed")
	}
	if got := harness.lastInfo(); got != "Marked "+clientPath+" not viewed (a hunk is no longer reviewed)" {
		test.Errorf("status = %q", got)
	}
	if got := viewedSent(harness.client); len(got) != 2 || got[1] {
		test.Errorf("GitHub got %v, want viewed=true then false", got)
	}
}

// TestAutoViewedLeavesUserMarks: a file the user marked with v is never
// unmarked automatically, whether v came before the hunks or after strig
// marked it.
func TestAutoViewedLeavesUserMarks(test *testing.T) {
	before := autoHarness(test, true, github.FileViewed)
	before.press("m", "]", "m", "m")
	if before.clientFile() != github.FileViewed || len(viewedSent(before.client)) != 0 {
		test.Error("autoviewed changed a file the user had marked viewed")
	}

	after := autoHarness(test, true)
	after.press("m", "]", "m") // strig marks it
	after.press("v", "v")      // the user unmarks and marks it again
	after.press("m")
	if after.clientFile() != github.FileViewed {
		test.Error("unreviewing a hunk unmarked a file the user had marked with v")
	}
}

// TestAutoViewedAcrossWorkspaces: the record that strig marked a file lives
// with the shared hunk marks, so reopening the pull request still unmarks it.
func TestAutoViewedAcrossWorkspaces(test *testing.T) {
	first := autoHarness(test, true)
	first.press("m", "]", "m")

	reopened := newHarness(test, 100, 30)
	reopened.client = first.client
	reopened.view.client = first.client
	WithHunkMarks(first.view.hunkMarks)(reopened.view)
	reopened.run(reopened.view.Init())
	reopened.send(userinterface.OptionsMessage{Values: options.Values{}.With(options.AutoViewed, true)})
	reopened.press("j", "l")
	if reopened.clientFile() != github.FileViewed {
		test.Fatal("setup: the reopened pull request does not show the file viewed")
	}
	reopened.press("m")
	if reopened.clientFile() == github.FileViewed {
		test.Error("unreviewing a hunk after reopening left the auto-marked file viewed")
	}
}

// TestAutoViewedFailedMark: when GitHub refuses, the file stays unviewed and
// unreviewing a hunk later sends nothing.
func TestAutoViewedFailedMark(test *testing.T) {
	harness := autoHarness(test, true)
	harness.client.SetError(fake.MethodSetFileViewed, errors.New("boom"))
	harness.press("m", "]", "m")
	if harness.clientFile() == github.FileViewed {
		test.Error("a refused mark left the file viewed")
	}
	harness.client.SetError(fake.MethodSetFileViewed, nil)
	harness.press("m")
	if got := viewedSent(harness.client); len(got) != 1 {
		test.Errorf("GitHub got %v, want only the refused mark", got)
	}
}

func TestAutoViewedWithoutPullRequestIdentifier(test *testing.T) {
	harness := autoHarness(test, true)
	harness.view.detail.Identifier = ""
	harness.press("m")
	if err := harness.lastErr(); err != nil {
		test.Errorf("reviewing a hunk that does not complete the file showed %v", err)
	}
	harness.press("]", "m")
	if err := harness.lastErr(); err == nil {
		test.Error("no error when the file could not be marked viewed")
	}
	if len(harness.view.hunkMarks.autoViewed) != 0 {
		test.Error("strig recorded a mark it never sent")
	}
}
