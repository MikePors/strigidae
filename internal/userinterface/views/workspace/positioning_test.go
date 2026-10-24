package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestScreenPositioningInTheDiffPane(test *testing.T) {
	harness := threadHarness(test, 100, 14)
	harness.press("}", "l", ")", "z o") // an expanded thread: its rows are rows of the view
	if !strings.Contains(ansi.Strip(harness.view.Render(100, 14)), "Nit: unused import.") {
		test.Fatal("the thread did not expand")
	}
	pane := harness.pane()
	harness.press("g g")
	for range 16 {
		harness.press("j")
	}
	for range 4 {
		harness.press("k")
	}
	height := harness.view.window.Height() // the Files list and the diff pane are equally tall
	harness.view.Render(100, 14)
	cursor := pane.CursorRow()
	if above := cursor - pane.Top(); above != height/2 {
		test.Fatalf("start: %d rows above the cursor, want %d (cursor %d, top %d)", above, height/2, cursor, pane.Top())
	}
	tests := []struct {
		key       string
		wantAbove int
	}{{"z b", height - 1}, {"z z", (height - 1) / 2}, {"z t", 0}}
	for _, testCase := range tests {
		harness.press(testCase.key)
		harness.view.Render(100, 14)
		if got := pane.CursorRow(); got != cursor {
			test.Errorf("%s moved the cursor from %d to %d", testCase.key, cursor, got)
		}
		if got := cursor - pane.Top(); got != testCase.wantAbove {
			test.Errorf("%s: %d rows above the cursor, want %d", testCase.key, got, testCase.wantAbove)
		}
	}
}

func TestScreenPositioningInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, headerRows+2+3) // a three-row Files list over Overview and five files
	harness.view.Render(100, headerRows+2+3)
	if got, want := harness.view.window.Height(), 3; got != want {
		test.Fatalf("Files list height = %d, want %d", got, want)
	}
	harness.press("j", "j", "j") // the cursor is on row 3 of 6
	tests := []struct {
		key     string
		wantTop int
	}{{"z t", 3}, {"z z", 2}, {"z b", 1}}
	for _, testCase := range tests {
		harness.press(testCase.key)
		if got := harness.view.window.Cursor(); got != 3 {
			test.Errorf("%s moved the cursor to %d", testCase.key, got)
		}
		if got := harness.view.window.Top(); got != testCase.wantTop {
			test.Errorf("%s: top = %d, want %d", testCase.key, got, testCase.wantTop)
		}
	}
}
