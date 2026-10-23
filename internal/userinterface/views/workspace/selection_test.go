package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFilePaneFollowsSelection(t *testing.T) {
	h := loaded(t, 100, 30)
	h.press("j")
	if !strings.Contains(ansi.Strip(h.render()), "func (c *Client) Sync") {
		t.Error("selecting a file should show its diff")
	}
	if got := h.v.rightTitle(); got != "internal/sync/client.go" {
		t.Errorf("right pane title = %q", got)
	}
	h.press("g g")
	if got := h.v.rightTitle(); got != "Overview" {
		t.Errorf("right pane title = %q", got)
	}
}

func TestRefreshKeepsFilesListTop(t *testing.T) {
	h := loaded(t, 100, 7) // three file rows visible
	h.press("G", "k", "k")
	top, cur := h.v.fw.Top(), h.v.sel()
	if top != 3 || cur != 3 {
		t.Fatalf("setup: top=%d cursor=%d, want 3 and 3", top, cur)
	}
	h.press("R")
	if h.v.fw.Top() != top || h.v.sel() != cur {
		t.Errorf("refresh moved the list: top=%d cursor=%d, want %d and %d", h.v.fw.Top(), h.v.sel(), top, cur)
	}
}

func TestRefreshWithShorterListStaysInBounds(t *testing.T) {
	h := loaded(t, 100, 7)
	h.press("G")
	d := testDetail()
	d.Files = d.Files[:2]
	h.send(pullRequestLoadedMessage{owner: h.v, repository: testRepository, number: 7, sequence: h.v.sequence, detail: d})
	if h.v.sel() < 0 || h.v.sel() > 2 {
		t.Errorf("selection = %d after the list shrank", h.v.sel())
	}
	_ = h.render()
}
