package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFilePaneFollowsSelection(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j")
	if !strings.Contains(ansi.Strip(harness.render()), "func (c *Client) Sync") {
		test.Error("selecting a file should show its diff")
	}
	if got := harness.v.rightTitle(); got != "internal/sync/client.go" {
		test.Errorf("right pane title = %q", got)
	}
	harness.press("g g")
	if got := harness.v.rightTitle(); got != "Overview" {
		test.Errorf("right pane title = %q", got)
	}
}

func TestRefreshKeepsFilesListTop(test *testing.T) {
	harness := loaded(test, 100, 7) // three file rows visible
	harness.press("G", "k", "k")
	top, current := harness.v.fw.Top(), harness.v.selection()
	if top != 3 || current != 3 {
		test.Fatalf("setup: top=%d cursor=%d, want 3 and 3", top, current)
	}
	harness.press("R")
	if harness.v.fw.Top() != top || harness.v.selection() != current {
		test.Errorf("refresh moved the list: top=%d cursor=%d, want %d and %d", harness.v.fw.Top(), harness.v.selection(), top, current)
	}
}

func TestRefreshWithShorterListStaysInBounds(test *testing.T) {
	harness := loaded(test, 100, 7)
	harness.press("G")
	detail := testDetail()
	detail.Files = detail.Files[:2]
	harness.send(pullRequestLoadedMessage{owner: harness.v, repository: testRepository, number: 7, sequence: harness.v.sequence, detail: detail})
	if harness.v.selection() < 0 || harness.v.selection() > 2 {
		test.Errorf("selection = %d after the list shrank", harness.v.selection())
	}
	_ = harness.render()
}
