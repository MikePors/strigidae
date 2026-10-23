package diffview

import (
	"testing"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func parseFile(test *testing.T, body string) *diff.File {
	test.Helper()
	parsedDiff, err := diff.Parse("diff --git a/f b/f\n--- a/f\n+++ b/f\n" + body)
	if err != nil {
		test.Fatal(err)
	}
	return parsedDiff.Files[0]
}

const (
	firstVersion = "@@ -1,5 +1,5 @@\n a\n-b\n+B\n c\n d\n e\n"
	// v2: new commits added two lines above, so every row moved down by two.
	secondVersion = "@@ -1,5 +1,7 @@\n+x\n+y\n a\n-b\n+B\n c\n d\n e\n"
	// v3: the anchored deletion is gone; the nearest line is "b" as context.
	thirdVersion = "@@ -1,5 +1,5 @@\n a\n b\n c\n d\n e\n"
)

func TestReparseReanchorsOnSameLine(test *testing.T) {
	model := New(theme.New(true))
	model.SetFile(parseFile(test, firstVersion))
	model.SetSize(60, 4)
	model.CursorDown(2) // the deleted "b"
	if cursorLine, _ := model.Cursor(); cursorLine.Kind != diff.Delete || cursorLine.Old != 2 {
		test.Fatalf("setup: %+v", cursorLine)
	}

	model.SetFile(parseFile(test, secondVersion))
	cursorLine, ok := model.Cursor()
	if !ok || cursorLine.Kind != diff.Delete || cursorLine.Old != 2 || cursorLine.Text != "b" {
		test.Errorf("after rows moved: %+v ok=%v", cursorLine, ok)
	}
	if model.window.Cursor() != 4 {
		test.Errorf("cursor row = %d, want 4", model.window.Cursor())
	}
	if model.window.Cursor() < model.window.Top() || model.window.Cursor() >= model.window.Top()+4 {
		test.Errorf("cursor %d not visible from top %d", model.window.Cursor(), model.window.Top())
	}

	model.SetFile(parseFile(test, thirdVersion))
	cursorLine, ok = model.Cursor()
	if !ok || cursorLine.Text != "b" || cursorLine.Kind != diff.Context {
		test.Errorf("fallback to nearest line: %+v ok=%v", cursorLine, ok)
	}
}

func TestReparseKeepsHeaderAndMarkerAnchors(test *testing.T) {
	const withMarker = "@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+B\n@@ -9 +9 @@\n-z\n+Z\n"
	model := New(theme.New(true))
	model.SetFile(parseFile(test, withMarker))
	model.SetSize(60, 10)
	model.NextHunk()
	model.SetFile(parseFile(test, withMarker))
	if model.window.Cursor() != 5 {
		test.Errorf("header anchor: cursor = %d", model.window.Cursor())
	}
	model.GotoTop()
	model.CursorDown(3) // the marker row
	model.SetFile(parseFile(test, withMarker))
	if cursorLine, ok := model.Cursor(); !ok || cursorLine.Text != "b" {
		test.Errorf("marker anchors to its line: %+v ok=%v", cursorLine, ok)
	}
}

func TestMessageKeepsSavedPosition(test *testing.T) {
	model := New(theme.New(true))
	model.SetFile(parseFile(test, firstVersion))
	model.SetSize(60, 4)
	model.CursorDown(2)

	model.SetMessage("Loading diff...")
	if model.Render(60, 4) == "" {
		test.Error("message not rendered")
	}
	model.SetFile(parseFile(test, secondVersion))
	if cursorLine, ok := model.Cursor(); !ok || cursorLine.Kind != diff.Delete || cursorLine.Old != 2 {
		test.Errorf("position lost across SetMessage: %+v ok=%v", cursorLine, ok)
	}

	// An error shown mid-refresh keeps it too.
	model.SetDiffError(errTest)
	model.SetMessage("still loading")
	model.SetFile(parseFile(test, secondVersion))
	if cursorLine, ok := model.Cursor(); !ok || cursorLine.Old != 2 {
		test.Errorf("position lost across error: %+v ok=%v", cursorLine, ok)
	}

	// A different file after a message starts fresh.
	model.SetMessage("Loading diff...")
	other := &diff.File{NewPath: "g", OldPath: "g", Hunks: parseFile(test, firstVersion).Hunks}
	model.SetFile(other)
	if model.window.Cursor() != 0 {
		test.Errorf("different path should start at 0, got %d", model.window.Cursor())
	}
	// And an explicit clear forgets the saved position.
	model.SetFile(parseFile(test, secondVersion))
	model.CursorDown(4)
	model.SetMessage("x")
	model.SetFile(nil)
	model.SetFile(parseFile(test, secondVersion))
	if model.window.Cursor() != 0 {
		test.Errorf("SetFile(nil) should drop the saved position, got %d", model.window.Cursor())
	}
}

type testErr struct{}

func (testErr) Error() string { return "boom" }

var errTest error = testErr{}
