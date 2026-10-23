package picker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestLongQueryAndPathsStayOnOneRow(test *testing.T) {
	long := "internal/ui/views/workspace/testdata/some/very/deep/directory/structure/file.go"
	model := New(theme.Default(), "Files", []string{long})
	out := ansi.Strip(model.Render(40, 12))
	if !strings.Contains(out, "file.go") {
		test.Errorf("file name should stay visible:\n%s", out)
	}
	base := len(strings.Split(out, "\n"))

	typeText(model, strings.Repeat("i", 70))
	lines := strings.Split(ansi.Strip(model.Render(40, 12)), "\n")
	if len(lines) > base+1 { // +1: the "no matches" row replaces nothing else
		test.Errorf("long query added rows: %d vs %d", len(lines), base)
	}
	if !strings.HasPrefix(lines[1], "│> …") {
		test.Errorf("query row should scroll sideways, got %q", lines[1])
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 40 {
			test.Errorf("line %q wider than 40", line)
		}
	}
}
