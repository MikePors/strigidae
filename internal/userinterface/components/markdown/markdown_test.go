package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestRenderCachesAndTrims(test *testing.T) {
	renderer := New(theme.New(true))
	out := renderer.Render("# Title\n\nsome **bold** text", 40)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Title") || !strings.Contains(plain, "bold") {
		test.Errorf("unexpected output %q", plain)
	}
	if strings.HasPrefix(out, "\n") || strings.HasSuffix(out, "\n") {
		test.Error("surrounding blank lines should be trimmed")
	}
	if len(renderer.cache) != 1 {
		test.Fatalf("cache size = %d", len(renderer.cache))
	}
	if again := renderer.Render("# Title\n\nsome **bold** text", 40); again != out {
		test.Error("cached render differs")
	}
	renderer.Render("# Title\n\nsome **bold** text", 60)
	if len(renderer.cache) != 1 {
		test.Errorf("width should be part of the cache key, size = %d", len(renderer.cache))
	}
}

func TestStyleIsFixed(test *testing.T) {
	if New(theme.New(true)).Render("**x**", 20) == New(theme.New(false)).Render("**x**", 20) {
		test.Error("dark and light styles should differ")
	}
}

func TestWraps(test *testing.T) {
	out := ansi.Strip(New(theme.New(true)).Render(strings.Repeat("word ", 40), 30))
	for _, line := range strings.Split(out, "\n") {
		if ansi.StringWidth(line) > 30 {
			test.Errorf("line %q wider than 30", line)
		}
	}
}
