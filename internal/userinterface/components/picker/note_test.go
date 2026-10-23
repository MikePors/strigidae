package picker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestNoteIsDrawnWrappedWithinTheBox(test *testing.T) {
	model := New(theme.Default(), "Title", []string{"one"})
	model.SetNote("You can't approve or request changes on your own pull request.")
	lines := strings.Split(ansi.Strip(model.Render(40, 20)), "\n")
	joined := strings.Join(lines, " ")
	for _, word := range []string{"You", "approve", "request."} {
		if !strings.Contains(joined, word) {
			test.Errorf("note lacks %q:\n%s", word, strings.Join(lines, "\n"))
		}
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 40 {
			test.Errorf("line wider than the box: %q", line)
		}
	}
}
