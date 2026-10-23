package picker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

const testNote = "You can't approve or request changes on your own pull request."

func TestNoteIsDrawnWrappedWithinTheBox(test *testing.T) {
	model := New(theme.Default(), "Title", []string{"one"})
	model.SetNote(testNote)
	lines := strings.Split(ansi.Strip(model.Render(40, 20)), "\n")
	joined := strings.Join(lines, " ")
	for _, word := range []string{"You", "approve", "request."} {
		if !strings.Contains(joined, word) {
			test.Errorf("note lacks %q:\n%s", word, strings.Join(lines, "\n"))
		}
	}
}

func TestNoteNeverCrowdsOutTheSelectedItem(test *testing.T) {
	tests := []struct {
		width, height int
		wantEmpty     bool
	}{
		{1, 1, true},
		{12, 5, false},
		{20, 6, false},
		{64, 5, false},
		{64, 6, false},
		{30, 8, false},
		{40, 20, false},
		{200, 50, false},
	}
	for _, testCase := range tests {
		model := New(theme.Default(), "Submit review", []string{"Comment"})
		model.SetNote(testNote)
		output := ansi.Strip(model.Render(testCase.width, testCase.height))
		if testCase.wantEmpty {
			if output != "" {
				test.Errorf("%dx%d: want no output, got %q", testCase.width, testCase.height, output)
			}
			continue
		}
		lines := strings.Split(output, "\n")
		if len(lines) > testCase.height {
			test.Errorf("%dx%d: %d lines is taller than the space", testCase.width, testCase.height, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > testCase.width {
				test.Errorf("%dx%d: line wider than the space: %q", testCase.width, testCase.height, line)
			}
		}
		if !strings.Contains(output, "> Comment") && !strings.Contains(output, "> Comme") && !strings.Contains(output, "> …") {
			test.Errorf("%dx%d: selected item not drawn:\n%s", testCase.width, testCase.height, output)
		}
	}
}
