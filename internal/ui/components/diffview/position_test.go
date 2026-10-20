package diffview

import (
	"testing"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

func parseFile(t *testing.T, body string) *diff.File {
	t.Helper()
	d, err := diff.Parse("diff --git a/f b/f\n--- a/f\n+++ b/f\n" + body)
	if err != nil {
		t.Fatal(err)
	}
	return d.Files[0]
}

const (
	v1 = "@@ -1,5 +1,5 @@\n a\n-b\n+B\n c\n d\n e\n"
	// v2: new commits added two lines above, so every row moved down by two.
	v2 = "@@ -1,5 +1,7 @@\n+x\n+y\n a\n-b\n+B\n c\n d\n e\n"
	// v3: the anchored deletion is gone; the nearest line is "b" as context.
	v3 = "@@ -1,5 +1,5 @@\n a\n b\n c\n d\n e\n"
)

func TestReparseReanchorsOnSameLine(t *testing.T) {
	m := New(theme.New(true))
	m.SetFile(parseFile(t, v1))
	m.SetSize(60, 4)
	m.CursorDown(2) // the deleted "b"
	if c, _ := m.Cursor(); c.Kind != diff.Delete || c.Old != 2 {
		t.Fatalf("setup: %+v", c)
	}

	m.SetFile(parseFile(t, v2))
	c, ok := m.Cursor()
	if !ok || c.Kind != diff.Delete || c.Old != 2 || c.Text != "b" {
		t.Errorf("after rows moved: %+v ok=%v", c, ok)
	}
	if m.cursor != 4 {
		t.Errorf("cursor row = %d, want 4", m.cursor)
	}
	if m.cursor < m.top || m.cursor >= m.top+4 {
		t.Errorf("cursor %d not visible from top %d", m.cursor, m.top)
	}

	m.SetFile(parseFile(t, v3))
	c, ok = m.Cursor()
	if !ok || c.Text != "b" || c.Kind != diff.Context {
		t.Errorf("fallback to nearest line: %+v ok=%v", c, ok)
	}
}

func TestReparseKeepsHeaderAndMarkerAnchors(t *testing.T) {
	const withMarker = "@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+B\n@@ -9 +9 @@\n-z\n+Z\n"
	m := New(theme.New(true))
	m.SetFile(parseFile(t, withMarker))
	m.SetSize(60, 10)
	m.NextHunk()
	m.SetFile(parseFile(t, withMarker))
	if m.cursor != 5 {
		t.Errorf("header anchor: cursor = %d", m.cursor)
	}
	m.GotoTop()
	m.CursorDown(3) // the marker row
	m.SetFile(parseFile(t, withMarker))
	if c, ok := m.Cursor(); !ok || c.Text != "b" {
		t.Errorf("marker anchors to its line: %+v ok=%v", c, ok)
	}
}

func TestMessageKeepsSavedPosition(t *testing.T) {
	m := New(theme.New(true))
	m.SetFile(parseFile(t, v1))
	m.SetSize(60, 4)
	m.CursorDown(2)

	m.SetMessage("Loading diff...")
	if m.Render(60, 4) == "" {
		t.Error("message not rendered")
	}
	m.SetFile(parseFile(t, v2))
	if c, ok := m.Cursor(); !ok || c.Kind != diff.Delete || c.Old != 2 {
		t.Errorf("position lost across SetMessage: %+v ok=%v", c, ok)
	}

	// An error shown mid-refresh keeps it too.
	m.SetDiffError(errTest)
	m.SetMessage("still loading")
	m.SetFile(parseFile(t, v2))
	if c, ok := m.Cursor(); !ok || c.Old != 2 {
		t.Errorf("position lost across error: %+v ok=%v", c, ok)
	}

	// A different file after a message starts fresh.
	m.SetMessage("Loading diff...")
	other := &diff.File{NewPath: "g", OldPath: "g", Hunks: parseFile(t, v1).Hunks}
	m.SetFile(other)
	if m.cursor != 0 {
		t.Errorf("different path should start at 0, got %d", m.cursor)
	}
	// And an explicit clear forgets the saved position.
	m.SetFile(parseFile(t, v2))
	m.CursorDown(4)
	m.SetMessage("x")
	m.SetFile(nil)
	m.SetFile(parseFile(t, v2))
	if m.cursor != 0 {
		t.Errorf("SetFile(nil) should drop the saved position, got %d", m.cursor)
	}
}

type testErr struct{}

func (testErr) Error() string { return "boom" }

var errTest error = testErr{}
