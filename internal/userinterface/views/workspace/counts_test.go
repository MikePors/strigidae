package workspace

import (
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

func (harness *harness) pressCounted(sequence string, count int) {
	harness.test.Helper()
	harness.send(keys.Message{Sequence: sequence, Count: count})
}

func TestCountsInTheDiffPane(test *testing.T) {
	tests := []struct {
		name  string
		key   string
		n     int
		plain []string
	}{
		{"3j is three j", "j", 3, []string{"j", "j", "j"}},
		{"11j past the end clamps like repeated j", "j", 11, []string{"j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j"}},
		{"3G goes to the third row", "G", 3, []string{"j", "j"}},
		{"42G beyond the diff goes to the last row", "G", 42, []string{"G"}},
		{"2ctrl+d is two half pages", "ctrl+d", 2, []string{"ctrl+d", "ctrl+d"}},
		{"2ctrl+f is two pages", "ctrl+f", 2, []string{"ctrl+f", "ctrl+f"}},
		{"3] across hunks stops at the last hunk", "]", 3, []string{"]", "]", "]"}},
		{"5z l scrolls five steps", "z l", 5, []string{"z l", "z l", "z l", "z l", "z l"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			counted := openClientDiff(test, 100, 24)
			counted.pressCounted(testCase.key, testCase.n)
			plain := openClientDiff(test, 100, 24)
			plain.press(testCase.plain...)
			if got, want := counted.cursorText(), plain.cursorText(); got != want {
				test.Errorf("cursor on %q, want %q", got, want)
			}
			if got, want := counted.pane().HorizontalOffset(), plain.pane().HorizontalOffset(); got != want {
				test.Errorf("horizontal offset = %d, want %d", got, want)
			}
		})
	}
}

func TestCountedHunkJumpsLandOnTheHunk(test *testing.T) {
	harness := openClientDiff(test, 100, 24)
	harness.pressCounted("]", 3)
	harness.press("j")
	if !strings.Contains(harness.cursorText(), "c.conn.Ping") {
		test.Errorf("3] then j: cursor on %q, want the second hunk", harness.cursorText())
	}
	harness.pressCounted("[", 3)
	harness.press("j")
	if !strings.Contains(harness.cursorText(), `"context"`) {
		test.Errorf("3[ then j: cursor on %q, want the first hunk", harness.cursorText())
	}
}

func TestCountedHorizontalScroll(test *testing.T) {
	harness := openClientDiff(test, 100, 24)
	harness.pressCounted("z l", 2)
	if got := harness.pane().HorizontalOffset(); got != 2*horizontalScrollStep {
		test.Errorf("2z l: offset = %d, want %d", got, 2*horizontalScrollStep)
	}
	harness.pressCounted("z h", 2)
	if got := harness.pane().HorizontalOffset(); got != 0 {
		test.Errorf("2z h: offset = %d, want 0", got)
	}
}

func TestCountedFileNavigation(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.pressCounted("}", 2)
	if got := harness.view.selection(); got != 2 {
		test.Errorf("2}: selection = %d, want 2", got)
	}
	harness.pressCounted("{", 9999)
	if got := harness.view.selection(); got != 1 {
		test.Errorf("9999{: selection = %d, want 1 (never Overview)", got)
	}
	harness.pressCounted("}", 9999)
	if got := harness.view.selection(); got != len(harness.view.files()) {
		test.Errorf("9999}: selection = %d, want the last file", got)
	}
}

func TestCountsInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.pressCounted("j", 2)
	if got := harness.view.selection(); got != 2 {
		test.Errorf("2j: selection = %d, want 2", got)
	}
	harness.pressCounted("G", 1)
	if got := harness.view.selection(); got != 0 {
		test.Errorf("1G: selection = %d, want the first entry (Overview)", got)
	}
	harness.pressCounted("G", 99)
	if got := harness.view.selection(); got != len(harness.view.files()) {
		test.Errorf("99G: selection = %d, want the last entry", got)
	}
}

func TestCountOnAKeyThatIgnoresItActsOnce(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.pressCounted("l", 5)
	if harness.view.focus != paneRight {
		test.Error("5l should focus the right pane")
	}
	harness.pressCounted("h", 5)
	if harness.view.focus != paneFiles {
		test.Error("5h should focus the files pane")
	}
}

func TestCountsInTheOverview(test *testing.T) {
	harness := loaded(test, 100, 14)
	harness.press("l")
	harness.pressCounted("j", 4)
	if got := harness.view.overview.viewport.YOffset(); got != 4 {
		test.Errorf("4j: offset = %d, want 4", got)
	}
	harness.pressCounted("k", 9999)
	if got := harness.view.overview.viewport.YOffset(); got != 0 {
		test.Errorf("9999k: offset = %d, want 0", got)
	}
	harness.pressCounted("]", 9999)
	last := harness.view.overview.sections[len(harness.view.overview.sections)-1]
	if got := harness.view.overview.viewport.YOffset(); got > last {
		test.Errorf("9999]: offset = %d, past the last section %d", got, last)
	}
}
