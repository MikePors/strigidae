package workspace

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFocusAndFileNavigation(t *testing.T) {
	h := loaded(t, 100, 30)
	if h.v.focus != paneFiles || h.v.sel() != 0 {
		t.Fatalf("initial focus=%v sel=%d", h.v.focus, h.v.sel())
	}
	h.press("l")
	if h.v.focus != paneRight {
		t.Error("l should focus the right pane")
	}
	h.press("h")
	if h.v.focus != paneFiles {
		t.Error("h should focus the files pane")
	}

	h.press("j", "j")
	if h.v.sel() != 2 {
		t.Errorf("j j: sel = %d", h.v.sel())
	}
	h.press("k")
	if h.v.sel() != 1 {
		t.Errorf("k: sel = %d", h.v.sel())
	}
	h.press("G")
	if h.v.sel() != 5 {
		t.Errorf("G: sel = %d", h.v.sel())
	}
	h.press("j")
	if h.v.sel() != 5 {
		t.Errorf("j past the end: sel = %d", h.v.sel())
	}
	h.press("g g")
	if h.v.sel() != 0 {
		t.Errorf("g g: sel = %d", h.v.sel())
	}
	h.press("ctrl+f")
	if h.v.sel() != 5 {
		t.Errorf("ctrl+f: sel = %d", h.v.sel())
	}

	// enter in the Files pane focuses the right pane.
	h.press("enter")
	if h.v.focus != paneRight {
		t.Error("enter should focus the right pane")
	}

	// ] f and [ f work from either pane and never land on Overview.
	h.press("g g") // scrolls the overview, selection unchanged
	if h.v.sel() != 5 {
		t.Errorf("g g in the right pane moved the selection to %d", h.v.sel())
	}
	h.press("[ f")
	if h.v.sel() != 4 {
		t.Errorf("[ f: sel = %d", h.v.sel())
	}
	h.press("h", "g g", "[ f")
	if h.v.sel() != 0 {
		t.Errorf("[ f from Overview should stay: sel = %d", h.v.sel())
	}
	h.press("] f")
	if h.v.sel() != 1 {
		t.Errorf("] f from Overview: sel = %d", h.v.sel())
	}
	h.press("l", "] f", "] f", "] f", "] f", "] f")
	if h.v.sel() != 5 {
		t.Errorf("] f clamps at the last file: sel = %d", h.v.sel())
	}
}

func TestSectionJumps(t *testing.T) {
	h := loaded(t, 100, 14)
	h.press("l")
	if h.v.ov.vp.YOffset() != 0 {
		t.Fatal("overview should start at the top")
	}
	h.press("] s")
	conv := h.v.ov.sections[1]
	if got := h.v.ov.vp.YOffset(); got != conv {
		t.Errorf("] s: offset = %d, want %d", got, conv)
	}
	h.press("[ s")
	if got := h.v.ov.vp.YOffset(); got != 0 {
		t.Errorf("[ s: offset = %d, want 0", got)
	}
	// Works from the Files pane as well.
	h.press("h", "] s")
	if got := h.v.ov.vp.YOffset(); got != conv {
		t.Errorf("] s from files: offset = %d, want %d", got, conv)
	}
	// Ignored while a file is selected.
	h.press("j", "[ s")
	h.press("g g", "] s")
}

func TestOverviewScrolling(t *testing.T) {
	h := loaded(t, 100, 14)
	h.press("l", "j")
	if h.v.ov.vp.YOffset() != 1 {
		t.Errorf("j: offset = %d", h.v.ov.vp.YOffset())
	}
	h.press("k")
	h.press("ctrl+d")
	if h.v.ov.vp.YOffset() == 0 {
		t.Error("ctrl+d should scroll")
	}
	h.press("G")
	if !h.v.ov.vp.AtBottom() {
		t.Error("G should reach the bottom")
	}
	h.press("g g")
	if !h.v.ov.vp.AtTop() {
		t.Error("g g should reach the top")
	}
	h.press("ctrl+f", "ctrl+b", "ctrl+u")
}

func TestOpenInBrowser(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("o")
	if diff := cmp.Diff([]string{"https://example.test/acme/widgets/pull/7"}, h.opened); diff != "" {
		t.Errorf("opened (-want +got):\n%s", diff)
	}

	h2 := newHarness(t, 100, 24) // not loaded yet
	h2.press("o")
	if len(h2.opened) != 0 || len(h2.msgs) == 0 {
		t.Errorf("o before load: opened=%v msgs=%v", h2.opened, h2.msgs)
	}

	h3 := loaded(t, 100, 24)
	h3.v.open = func(string) error { return errors.New("no browser") }
	h3.press("o")
	if h3.lastErr() == nil {
		t.Error("browser failure should reach the status bar")
	}
}
