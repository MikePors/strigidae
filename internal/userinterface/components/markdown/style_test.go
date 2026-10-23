package markdown

import (
	"fmt"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// sgrRGB is the tail of a true-colour SGR sequence for c.
func sgrRGB(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestHeadingsKeepMutedMarkers(t *testing.T) {
	th := theme.New(true)
	out := New(th).Render("# One\n\n## Two\n\n### Three\n\n#### Four", 40)
	plain := ansi.Strip(out)
	for _, w := range []string{"## Two", "### Three", "#### Four"} {
		if !strings.Contains(plain, w) {
			t.Errorf("heading %q missing:\n%s", w, plain)
		}
	}
	if !strings.Contains(plain, "One") {
		t.Errorf("h1 missing:\n%s", plain)
	}
	// The marker is muted and the text takes the level's colour.
	muted, accent := sgrRGB(th.Roles.Muted), sgrRGB(th.Roles.Accent)
	i := strings.Index(out, "##")
	j := strings.Index(out, "Two")
	if i < 0 || j < i {
		t.Fatalf("marker/text not found in %q", out)
	}
	if !strings.Contains(out[:i], muted) {
		t.Errorf("marker is not drawn in the muted role %s: %q", muted, out)
	}
	if !strings.Contains(out[i:j], accent) {
		t.Errorf("heading text is not in the accent role %s: %q", accent, out)
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
			"string": th.Roles.SyntaxString,
		} {
			if rgb := sgrRGB(c); !strings.Contains(out, rgb) {
				t.Errorf("%s theme: output lacks the %s role colour %s: %q", th.Identifier, name, rgb, out)
			}
		}
	}
}

// TestOverrideColoursCodeBlocks: a syntax role override reaches rendered
// Markdown code, not just the first theme registered.
func TestOverrideColoursCodeBlocks(t *testing.T) {
	const md = "```go\nvar s = \"x\"\n```"
	p := testPalette(t)
	base := theme.FromPalette(p)
	over := theme.FromPaletteWithOverrides(p, theme.Roles{SyntaxString: color.RGBA{R: 255, B: 255, A: 255}})
	if out := New(base).Render(md, 40); strings.Contains(out, "2;255;0;255") {
		t.Fatalf("setup: default theme already uses the override colour: %q", out)
	}
	if out := New(over).Render(md, 40); !strings.Contains(out, "2;255;0;255") {
		t.Errorf("overridden string colour missing from the rendered code block: %q", out)
	}
}

// TestStyleConfigFollowsPalette builds the Glamour style from two very
// different palettes and checks every colour in it changes.
func TestStyleConfigFollowsPalette(t *testing.T) {
	loud := testPalette(t)
	a, b := collectColours(styleConfig(theme.New(true))), collectColours(styleConfig(theme.FromPalette(loud)))
	if len(a) < 15 {
		t.Fatalf("only %d colours found in the Glamour style; the walk is broken", len(a))
	}
	for path, ca := range a {
		if cb, ok := b[path]; !ok || ca == cb {
			t.Errorf("%s is %q in both palettes (or missing); is it hardcoded?", path, ca)
		}
	}
}

// collectColours returns every non-nil *string field named Color or
// BackgroundColor in v, by path.
func collectColours(v any) map[string]string {
	out := map[string]string{}
	var walk func(reflect.Value, string)
	walk = func(rv reflect.Value, path string) {
		if rv.Kind() != reflect.Struct {
			return
		}
		for i := range rv.NumField() {
			f, name := rv.Field(i), path+"."+rv.Type().Field(i).Name
			switch {
			case f.Kind() == reflect.Struct:
				walk(f, name)
			case f.Kind() == reflect.Pointer && f.Type().Elem().Kind() == reflect.String && !f.IsNil():
				if n := rv.Type().Field(i).Name; n == "Color" || n == "BackgroundColor" {
					out[name] = f.Elem().String()
				}
			}
		}
	}
	walk(reflect.ValueOf(v), "")
	return out
}

func testPalette(t *testing.T) theme.Palette {
	t.Helper()
	var b16 [16]color.Color
	for i := range b16 {
		b16[i] = color.RGBA{R: uint8(255 - i*13), G: uint8(30 + i*11), B: uint8(7 + i*9), A: 255}
	}
	p, err := theme.FromBase16("loud", theme.Dark, b16)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
