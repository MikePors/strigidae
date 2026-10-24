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

// newSideLine returns the new-side line number under the diff cursor.
func (harness *harness) newSideLine() int {
	harness.test.Helper()
	cursorLine, ok := harness.pane().Cursor()
	if !ok {
		harness.test.Fatal("cursor is not on a diff line")
	}
	return cursorLine.New
}

// The client.go diff shows new-side lines 10-19 (hunk 1) and 42-48 (hunk 2).
func TestGotoLineInTheDiffPane(test *testing.T) {
	tests := []struct {
		name string
		line int
		want int
	}{
		{"exact hit in the first hunk", 12, 12},
		{"exact hit in the second hunk", 42, 42},
		{"line between hunks goes to the closer line", 30, 19},
		{"line before the first hunk goes to its first line", 3, 10},
		{"line past the end goes to the last line", 500, 48},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := openClientDiff(test, 100, 24)
			harness.pressCounted("G", testCase.line)
			if got := harness.newSideLine(); got != testCase.want {
				test.Errorf("%dG landed on line %d, want %d", testCase.line, got, testCase.want)
			}
		})
	}
}

func TestCountsInTheDiffPane(test *testing.T) {
	tests := []struct {
		name  string
		key   string
		count int
		plain []string
	}{
		{"3j is three j", "j", 3, []string{"j", "j", "j"}},
		{"11j is eleven j", "j", 11, []string{"j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j"}},
		{"2ctrl+d is two half pages", "ctrl+d", 2, []string{"ctrl+d", "ctrl+d"}},
		{"2ctrl+f is two pages", "ctrl+f", 2, []string{"ctrl+f", "ctrl+f"}},
		{"5z l scrolls five steps", "z l", 5, []string{"z l", "z l", "z l", "z l", "z l"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			counted := openClientDiff(test, 100, 24)
			counted.pressCounted(testCase.key, testCase.count)
			plain := openClientDiff(test, 100, 24)
			plain.press(testCase.plain...)
			if got, want := counted.cursorText(), plain.cursorText(); got != want {
				test.Errorf("cursor on %q, want %q", got, want)
			}
			if got, want := counted.pane().CursorRow(), plain.pane().CursorRow(); got != want {
				test.Errorf("cursor row = %d, want %d", got, want)
			}
			if got, want := counted.pane().HorizontalOffset(), plain.pane().HorizontalOffset(); got != want {
				test.Errorf("horizontal offset = %d, want %d", got, want)
			}
		})
	}
}

func TestCountedHunkJumps(test *testing.T) {
	harness := openClientDiff(test, 100, 24)
	harness.pressCounted("]", 3) // only two hunks: stops on the last
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
	harness.pressCounted("o", 5)
	if len(harness.opened) != 1 {
		test.Errorf("5o opened the browser %d times, want 1", len(harness.opened))
	}
}

func TestCountsInTheOverview(test *testing.T) {
	// The Overview has sections at 0, 31, 46 and 49; at this height the last one
	// is clamped, so each case is compared with the same keys typed plainly.
	tests := []struct {
		name  string
		key   string
		count int
		plain []string
	}{
		{"4j", "j", 4, []string{"j", "j", "j", "j"}},
		{"2]", "]", 2, []string{"]", "]"}},
		{"9999]", "]", 9999, []string{"]", "]", "]"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			counted := loaded(test, 100, 8)
			counted.press("l")
			counted.pressCounted(testCase.key, testCase.count)
			plain := loaded(test, 100, 8)
			plain.press("l")
			plain.press(testCase.plain...)
			got, want := counted.view.overview.viewport.YOffset(), plain.view.overview.viewport.YOffset()
			if got != want {
				test.Errorf("offset = %d, want %d", got, want)
			}
			plain.view.overview.viewport.SetYOffset(0)
			plain.press("]")
			if once := plain.view.overview.viewport.YOffset(); testCase.count > 1 && got == once {
				test.Errorf("offset %d equals a single step: the count had no effect", got)
			}
		})
	}
}
