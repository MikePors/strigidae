package workspace

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func (harness *harness) pane() *diffview.Model { return harness.v.filePane.(*diffPane).dv }

// cursorText returns the text of the diff line under the cursor ("" on a hunk header).
func (harness *harness) cursorText() string {
	cursorLine, ok := harness.pane().Cursor()
	if !ok {
		return ""
	}
	return cursorLine.Text
}

// openClientDiff selects client.go and focuses the right pane.
func openClientDiff(test *testing.T, width, height int) *harness {
	harness := loaded(test, width, height)
	harness.press("}", "l")
	if got := harness.v.selectedPath(); got != "internal/sync/client.go" {
		test.Fatalf("selected %q", got)
	}
	return harness
}

func TestDiffPaneKeys(test *testing.T) {
	harness := openClientDiff(test, 100, 24)
	diffView := harness.pane()

	harness.press("j")
	if !strings.Contains(harness.cursorText(), `"context"`) {
		test.Errorf("j: cursor on %q", harness.cursorText())
	}
	harness.press("k")
	if harness.cursorText() != "" {
		test.Errorf("k: cursor on %q, want the hunk header", harness.cursorText())
	}
	harness.press("G")
	if !strings.Contains(harness.cursorText(), "}") {
		test.Errorf("G: cursor on %q", harness.cursorText())
	}
	harness.press("g g")
	if harness.cursorText() != "" {
		test.Errorf("g g: cursor on %q", harness.cursorText())
	}

	harness.press("ctrl+d")
	if !strings.Contains(harness.cursorText(), "})") {
		test.Errorf("ctrl+d: cursor on %q", harness.cursorText())
	}
	harness.press("ctrl+u")
	if harness.cursorText() != "" {
		test.Errorf("ctrl+u: cursor on %q", harness.cursorText())
	}
	harness.press("ctrl+f")
	if !strings.Contains(harness.cursorText(), "}") {
		test.Errorf("ctrl+f: cursor on %q", harness.cursorText())
	}
	harness.press("ctrl+b")
	if harness.cursorText() != "" {
		test.Errorf("ctrl+b: cursor on %q", harness.cursorText())
	}

	// Hunks: ] goes to the second hunk's header, [ back to the first.
	harness.press("]", "j")
	if !strings.Contains(harness.cursorText(), "c.conn.Ping") {
		test.Errorf("]: cursor on %q", harness.cursorText())
	}
	harness.press("[", "[", "j")
	if !strings.Contains(harness.cursorText(), `"context"`) {
		test.Errorf("[: cursor on %q", harness.cursorText())
	}

	// Horizontal scrolling: 4 columns, then half a screen.
	harness.press("z l")
	if diffView.HorizontalOffset() != 4 {
		test.Errorf("z l: offset = %d, want 4", diffView.HorizontalOffset())
	}
	harness.press("z h")
	if diffView.HorizontalOffset() != 0 {
		test.Errorf("z h: offset = %d, want 0", diffView.HorizontalOffset())
	}
	harness.press("z L")
	half := diffView.HorizontalOffset()
	if half <= 4 {
		test.Errorf("z L: offset = %d, want half a screen", half)
	}
	harness.press("z H")
	if diffView.HorizontalOffset() != 0 {
		test.Errorf("z H: offset = %d, want 0", diffView.HorizontalOffset())
	}
}

func TestDiffPaneKeysReachOnlyWhenRightFocused(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.press("}") // Files pane still focused
	harness.press("j") // moves the file selection, not the diff cursor
	if got := harness.v.selectedPath(); got != "internal/sync/retry.go" {
		test.Errorf("selected %q", got)
	}
}

func TestDiffLookupByOldPathAndRename(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.press("}", "}", "}") // legacy.go, deleted
	if got := harness.v.selectedPath(); got != "internal/sync/legacy.go" {
		test.Fatalf("selected %q", got)
	}
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "retryForever") {
		test.Errorf("deleted file diff missing:\n%s", out)
	}
	harness.press("}") // renamed file: ChangedFile carries the new path
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "New text.") {
		test.Errorf("renamed file diff missing:\n%s", out)
	}
}

func TestDiffPaneLookupFallsBackToOldPath(test *testing.T) {
	// A ChangedFile reporting the old path of a renamed file still finds it.
	pane := newDiffPane(theme.New(true), func() time.Time { return testNow })
	harness := loaded(test, 100, 24)
	pane.SetDiff(harness.v.diff)
	pane.SetFile(github.ChangedFile{Path: "docs/old-name.md", Change: github.FileRenamed})
	pane.SetSize(80, 10)
	if out := ansi.Strip(pane.Render(80, 10)); !strings.Contains(out, "New text.") {
		test.Errorf("old-path lookup failed:\n%s", out)
	}
}

func TestDiffPaneStates(test *testing.T) {
	test.Run("unknown file", func(test *testing.T) {
		harness := loaded(test, 100, 24)
		pane := newDiffPane(theme.New(true), func() time.Time { return testNow })
		pane.SetDiff(harness.v.diff)
		pane.SetFile(github.ChangedFile{Path: "nope.go"})
		if out := ansi.Strip(pane.Render(60, 5)); !strings.Contains(out, "No diff available") {
			test.Errorf("got %q", out)
		}
	})
	test.Run("loading", func(test *testing.T) {
		harness := newHarness(test, 100, 24)
		harness.v.Init()
		harness.send(pullRequestLoadedMessage{owner: harness.v, repository: testRepository, number: 7, sequence: harness.v.sequence, detail: testDetail()})
		harness.press("}")
		golden.Assert(test, "diff_loading", harness.render(), false)
	})
	test.Run("too large", func(test *testing.T) {
		harness := newHarness(test, 100, 24)
		harness.client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
		harness.run(harness.v.Init())
		harness.press("}")
		golden.Assert(test, "diff_too_large", harness.render(), false)
	})
	test.Run("error", func(test *testing.T) {
		harness := newHarness(test, 100, 24)
		harness.client.SetError(fake.MethodGetDiff, errors.New("connection reset"))
		harness.run(harness.v.Init())
		harness.press("}")
		golden.Assert(test, "diff_error", harness.render(), false)
	})
	test.Run("parse error", func(test *testing.T) {
		harness := newHarness(test, 100, 24)
		harness.client.SetDiff(testRepository, 7, "diff --git a/x b/x\n@@ -1,2 +1,2 @@\n+only one line\n")
		harness.run(harness.v.Init())
		harness.press("}")
		out := ansi.Strip(harness.render())
		if !strings.Contains(out, "No diff available for this file.") || !strings.Contains(out, "could not be fully parsed") {
			test.Errorf("a file missing from a partly parsed diff should say why:\n%s", out)
		}
	})
}

func TestGoldenDiffPane(test *testing.T) {
	test.Run("populated", func(test *testing.T) {
		harness := openClientDiff(test, 100, 24)
		harness.press("j", "j")
		golden.Assert(test, "diff_populated", harness.render(), false)
	})
	test.Run("hscroll", func(test *testing.T) {
		harness := openClientDiff(test, 100, 24)
		harness.press("]", "j", "j", "j", "z L")
		golden.Assert(test, "diff_hscroll", harness.render(), false)
	})
	test.Run("deleted", func(test *testing.T) {
		harness := loaded(test, 100, 24)
		harness.press("}", "}", "}")
		golden.Assert(test, "diff_deleted", harness.render(), false)
	})
}
