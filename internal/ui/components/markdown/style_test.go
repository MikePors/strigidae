package markdown

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/ui/theme"
)

// sgrRGB is the tail of a true-colour SGR sequence for c.
func sgrRGB(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestHeadingsHaveNoMarkers(t *testing.T) {
	out := ansi.Strip(New(theme.New(true)).Render("# One\n\n## Two\n\n### Three\n\n#### Four", 40))
	if strings.Contains(out, "#") {
		t.Errorf("heading markers shown:\n%s", out)
	}
	for _, w := range []string{"One", "Two", "Three", "Four"} {
		if !strings.Contains(out, w) {
			t.Errorf("heading %q missing:\n%s", w, out)
		}
	}
}

func TestInlineCodeIsAMutedSurface(t *testing.T) {
	th := theme.New(true)
	out := New(th).Render("run `go test` now", 40)
	if !strings.Contains(out, "go test") {
		t.Fatalf("code missing: %q", out)
	}
	if rgb := sgrRGB(th.Roles.Error); strings.Contains(out, rgb) {
		t.Errorf("inline code uses the error colour %s: %q", rgb, out)
	}
	if rgb := sgrRGB(th.Roles.Surface); !strings.Contains(out, rgb) {
		t.Errorf("inline code lacks the surface background %s: %q", rgb, out)
	}
}

func TestOutputFollowsTheme(t *testing.T) {
	const md = "[link](https://example.com)\n\n```go\nvar s = \"x\"\n```"
	for _, th := range []theme.Theme{theme.New(true), theme.New(false)} {
		out := New(th).Render(md, 60)
		for name, c := range map[string]color.Color{
			"link":   th.Roles.Link,
			"string": th.Roles.SynString,
		} {
			if rgb := sgrRGB(c); !strings.Contains(out, rgb) {
				t.Errorf("%s theme: output lacks the %s role colour %s: %q", th.ID, name, rgb, out)
			}
		}
	}
}
