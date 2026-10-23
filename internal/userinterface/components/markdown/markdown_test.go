package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestRenderCachesAndTrims(t *testing.T) {
	r := New(theme.New(true))
	out := r.Render("# Title\n\nsome **bold** text", 40)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Title") || !strings.Contains(plain, "bold") {
		t.Errorf("unexpected output %q", plain)
	}
	if strings.HasPrefix(out, "\n") || strings.HasSuffix(out, "\n") {
		t.Error("surrounding blank lines should be trimmed")
	}
	if len(r.cache) != 1 {
		t.Fatalf("cache size = %d", len(r.cache))
	}
	if again := r.Render("# Title\n\nsome **bold** text", 40); again != out {
		t.Error("cached render differs")
	}
	r.Render("# Title\n\nsome **bold** text", 60)
	if len(r.cache) != 1 {
		t.Errorf("width should be part of the cache key, size = %d", len(r.cache))
	}
}

func TestStyleIsFixed(t *testing.T) {
	if New(theme.New(true)).Render("**x**", 20) == New(theme.New(false)).Render("**x**", 20) {
		t.Error("dark and light styles should differ")
	}
}

func TestWraps(t *testing.T) {
	out := ansi.Strip(New(theme.New(true)).Render(strings.Repeat("word ", 40), 30))
	for _, l := range strings.Split(out, "\n") {
		if ansi.StringWidth(l) > 30 {
			t.Errorf("line %q wider than 30", l)
		}
	}
}
