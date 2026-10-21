package picker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/ui/theme"
)

func TestLongQueryAndPathsStayOnOneRow(t *testing.T) {
	long := "internal/ui/views/workspace/testdata/some/very/deep/directory/structure/file.go"
	m := New(theme.Default(), "Files", []string{long})
	out := ansi.Strip(m.Render(40, 12))
	if !strings.Contains(out, "file.go") {
		t.Errorf("file name should stay visible:\n%s", out)
	}
	base := len(strings.Split(out, "\n"))

	typ(m, strings.Repeat("i", 70))
	lines := strings.Split(ansi.Strip(m.Render(40, 12)), "\n")
	if len(lines) > base+1 { // +1: the "no matches" row replaces nothing else
		t.Errorf("long query added rows: %d vs %d", len(lines), base)
	}
	if !strings.HasPrefix(lines[1], "│> …") {
		t.Errorf("query row should scroll sideways, got %q", lines[1])
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 40 {
			t.Errorf("line %q wider than 40", l)
		}
	}
}
