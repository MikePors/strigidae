package picker

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

const (
	testNote    = "You can't approve or request changes on your own pull request."
	testLongURL = "See https://docs.example.test/en/pull-requests/reviewing-changes-in-pull-requests/about-reviews for details."
)

// noteModel has count items and the cursor on the last one.
func noteModel(count int, note string) (*Model, string) {
	items := make([]string, count)
	for i := range items {
		items[i] = fmt.Sprintf("item%02d", i)
	}
	model := New(theme.Default(), "Submit review", items)
	model.SetNote(note)
	for range count - 1 {
		model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	return model, items[count-1]
}

func TestNoteIsDrawnWrappedWithinTheBox(test *testing.T) {
	model, _ := noteModel(1, testNote)
	output := ansi.Strip(model.Render(40, 20))
	lines := strings.Split(output, "\n")
	for _, word := range []string{"You", "approve", "request."} {
		if !strings.Contains(strings.Join(lines, " "), word) {
			test.Errorf("note lacks %q:\n%s", word, output)
		}
	}
	boxWidth := ansi.StringWidth(lines[0])
	for _, line := range lines {
		if ansi.StringWidth(line) > boxWidth {
			test.Errorf("line wider than the box (%d): %q", boxWidth, line)
		}
	}
}

func TestNoteNeverCrowdsOutTheSelectedItem(test *testing.T) {
	test.Run("too small to draw", func(test *testing.T) {
		model, _ := noteModel(1, testNote)
		if output := model.Render(1, 1); output != "" {
			test.Errorf("want no output, got %q", output)
		}
	})
	for _, note := range []string{testNote, testLongURL} {
		for _, count := range []int{1, 20} {
			for _, width := range []int{12, 20, 30, 64} {
				for height := 5; height <= 16; height++ {
					model, selected := noteModel(count, note)
					output := ansi.Strip(model.Render(width, height))
					lines := strings.Split(output, "\n")
					name := fmt.Sprintf("%d items %dx%d note %.8q", count, width, height, note)
					if len(lines) > height {
						test.Errorf("%s: %d lines is taller than the space", name, len(lines))
					}
					for _, line := range lines {
						if ansi.StringWidth(line) > width {
							test.Errorf("%s: line wider than the space: %q", name, line)
						}
					}
					// Items come first: the note may not take rows they would have had.
					drawn := strings.Count(output, "item")
					if want := min(count, height-4, 12); drawn < want {
						test.Errorf("%s: %d item rows drawn, want %d", name, drawn, want)
					}
					if !strings.Contains(output, "> "+selected) {
						test.Errorf("%s: selected item %q not drawn:\n%s", name, selected, output)
					}
				}
			}
		}
	}
}
