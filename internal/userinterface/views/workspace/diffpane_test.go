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

func (h *harness) pane() *diffview.Model { return h.v.filePane.(*diffPane).dv }

// cursorText returns the text of the diff line under the cursor ("" on a hunk header).
func (h *harness) cursorText() string {
	c, ok := h.pane().Cursor()
	if !ok {
		return ""
	}
	return c.Text
}

// openClientDiff selects client.go and focuses the right pane.
func openClientDiff(t *testing.T, w, ht int) *harness {
	h := loaded(t, w, ht)
	h.press("}", "l")
	if got := h.v.selectedPath(); got != "internal/sync/client.go" {
		t.Fatalf("selected %q", got)
	}
	return h
}

func TestDiffPaneKeys(t *testing.T) {
	h := openClientDiff(t, 100, 24)
	dv := h.pane()

	h.press("j")
	if !strings.Contains(h.cursorText(), `"context"`) {
		t.Errorf("j: cursor on %q", h.cursorText())
	}
	h.press("k")
	if h.cursorText() != "" {
		t.Errorf("k: cursor on %q, want the hunk header", h.cursorText())
	}
	h.press("G")
	if !strings.Contains(h.cursorText(), "}") {
		t.Errorf("G: cursor on %q", h.cursorText())
	}
	h.press("g g")
	if h.cursorText() != "" {
		t.Errorf("g g: cursor on %q", h.cursorText())
	}

	h.press("ctrl+d")
	if !strings.Contains(h.cursorText(), "})") {
		t.Errorf("ctrl+d: cursor on %q", h.cursorText())
	}
	h.press("ctrl+u")
	if h.cursorText() != "" {
		t.Errorf("ctrl+u: cursor on %q", h.cursorText())
	}
	h.press("ctrl+f")
	if !strings.Contains(h.cursorText(), "}") {
		t.Errorf("ctrl+f: cursor on %q", h.cursorText())
	}
	h.press("ctrl+b")
	if h.cursorText() != "" {
		t.Errorf("ctrl+b: cursor on %q", h.cursorText())
	}

	// Hunks: ] goes to the second hunk's header, [ back to the first.
	h.press("]", "j")
	if !strings.Contains(h.cursorText(), "c.conn.Ping") {
		t.Errorf("]: cursor on %q", h.cursorText())
	}
	h.press("[", "[", "j")
	if !strings.Contains(h.cursorText(), `"context"`) {
		t.Errorf("[: cursor on %q", h.cursorText())
	}

	// Horizontal scrolling: 4 columns, then half a screen.
	h.press("z l")
	if dv.XOffset() != 4 {
		t.Errorf("z l: offset = %d, want 4", dv.XOffset())
	}
	h.press("z h")
	if dv.XOffset() != 0 {
		t.Errorf("z h: offset = %d, want 0", dv.XOffset())
	}
	h.press("z L")
	half := dv.XOffset()
	if half <= 4 {
		t.Errorf("z L: offset = %d, want half a screen", half)
	}
	h.press("z H")
	if dv.XOffset() != 0 {
		t.Errorf("z H: offset = %d, want 0", dv.XOffset())
	}
}

func TestDiffPaneKeysReachOnlyWhenRightFocused(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("}") // Files pane still focused
	h.press("j") // moves the file selection, not the diff cursor
	if got := h.v.selectedPath(); got != "internal/sync/retry.go" {
		t.Errorf("selected %q", got)
	}
}

func TestDiffLookupByOldPathAndRename(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("}", "}", "}") // legacy.go, deleted
	if got := h.v.selectedPath(); got != "internal/sync/legacy.go" {
		t.Fatalf("selected %q", got)
	}
	if out := ansi.Strip(h.render()); !strings.Contains(out, "retryForever") {
		t.Errorf("deleted file diff missing:\n%s", out)
	}
	h.press("}") // renamed file: ChangedFile carries the new path
	if out := ansi.Strip(h.render()); !strings.Contains(out, "New text.") {
		t.Errorf("renamed file diff missing:\n%s", out)
	}
}

func TestDiffPaneLookupFallsBackToOldPath(t *testing.T) {
	// A ChangedFile reporting the old path of a renamed file still finds it.
	p := newDiffPane(theme.New(true), func() time.Time { return testNow })
	d := loaded(t, 100, 24)
	p.SetDiff(d.v.diff)
	p.SetFile(github.ChangedFile{Path: "docs/old-name.md", Change: github.FileRenamed})
	p.SetSize(80, 10)
	if out := ansi.Strip(p.Render(80, 10)); !strings.Contains(out, "New text.") {
		t.Errorf("old-path lookup failed:\n%s", out)
	}
}

func TestDiffPaneStates(t *testing.T) {
	t.Run("unknown file", func(t *testing.T) {
		h := loaded(t, 100, 24)
		p := newDiffPane(theme.New(true), func() time.Time { return testNow })
		p.SetDiff(h.v.diff)
		p.SetFile(github.ChangedFile{Path: "nope.go"})
		if out := ansi.Strip(p.Render(60, 5)); !strings.Contains(out, "No diff available") {
			t.Errorf("got %q", out)
		}
	})
	t.Run("loading", func(t *testing.T) {
		h := newHarness(t, 100, 24)
		h.v.Init()
		h.send(prLoadedMsg{owner: h.v, repo: testRepo, number: 7, seq: h.v.seq, detail: testDetail()})
		h.press("}")
		golden.Assert(t, "diff_loading", h.render(), false)
	})
	t.Run("too large", func(t *testing.T) {
		h := newHarness(t, 100, 24)
		h.client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
		h.run(h.v.Init())
		h.press("}")
		golden.Assert(t, "diff_too_large", h.render(), false)
	})
	t.Run("error", func(t *testing.T) {
		h := newHarness(t, 100, 24)
		h.client.SetError(fake.MethodGetDiff, errors.New("connection reset"))
		h.run(h.v.Init())
		h.press("}")
		golden.Assert(t, "diff_error", h.render(), false)
	})
	t.Run("parse error", func(t *testing.T) {
		h := newHarness(t, 100, 24)
		h.client.SetDiff(testRepo, 7, "diff --git a/x b/x\n@@ -1,2 +1,2 @@\n+only one line\n")
		h.run(h.v.Init())
		h.press("}")
		out := ansi.Strip(h.render())
		if !strings.Contains(out, "No diff available for this file.") || !strings.Contains(out, "could not be fully parsed") {
			t.Errorf("a file missing from a partly parsed diff should say why:\n%s", out)
		}
	})
}

func TestGoldenDiffPane(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		h := openClientDiff(t, 100, 24)
		h.press("j", "j")
		golden.Assert(t, "diff_populated", h.render(), false)
	})
	t.Run("hscroll", func(t *testing.T) {
		h := openClientDiff(t, 100, 24)
		h.press("]", "j", "j", "j", "z L")
		golden.Assert(t, "diff_hscroll", h.render(), false)
	})
	t.Run("deleted", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("}", "}", "}")
		golden.Assert(t, "diff_deleted", h.render(), false)
	})
}
