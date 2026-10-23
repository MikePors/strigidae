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

// sgrRGB is the tail of a true-colour SGR sequence for colour.
func sgrRGB(colour color.Color) string {
	red, green, blue, _ := colour.RGBA()
	return fmt.Sprintf("2;%d;%d;%d", red>>8, green>>8, blue>>8)
}

func TestHeadingsKeepMutedMarkers(test *testing.T) {
	activeTheme := theme.New(true)
	out := New(activeTheme).Render("# One\n\n## Two\n\n### Three\n\n#### Four", 40)
	plain := ansi.Strip(out)
	for _, word := range []string{"## Two", "### Three", "#### Four"} {
		if !strings.Contains(plain, word) {
			test.Errorf("heading %q missing:\n%s", word, plain)
		}
	}
	if !strings.Contains(plain, "One") {
		test.Errorf("h1 missing:\n%s", plain)
	}
	// The marker is muted and the text takes the level's colour.
	muted, accent := sgrRGB(activeTheme.Roles.Muted), sgrRGB(activeTheme.Roles.Accent)
	i := strings.Index(out, "##")
	j := strings.Index(out, "Two")
	if i < 0 || j < i {
		test.Fatalf("marker/text not found in %q", out)
	}
	if !strings.Contains(out[:i], muted) {
		test.Errorf("marker is not drawn in the muted role %s: %q", muted, out)
	}
	if !strings.Contains(out[i:j], accent) {
		test.Errorf("heading text is not in the accent role %s: %q", accent, out)
	}
}

func TestInlineCodeIsAMutedSurface(test *testing.T) {
	activeTheme := theme.New(true)
	out := New(activeTheme).Render("run `go test` now", 40)
	if !strings.Contains(out, "go test") {
		test.Fatalf("code missing: %q", out)
	}
	if rgb := sgrRGB(activeTheme.Roles.Error); strings.Contains(out, rgb) {
		test.Errorf("inline code uses the error colour %s: %q", rgb, out)
	}
	if rgb := sgrRGB(activeTheme.Roles.Surface); !strings.Contains(out, rgb) {
		test.Errorf("inline code lacks the surface background %s: %q", rgb, out)
	}
}

func TestOutputFollowsTheme(test *testing.T) {
	const markdownSource = "[link](https://example.com)\n\n```go\nvar s = \"x\"\n```"
	for _, activeTheme := range []theme.Theme{theme.New(true), theme.New(false)} {
		out := New(activeTheme).Render(markdownSource, 60)
		for name, colour := range map[string]color.Color{
			"link":   activeTheme.Roles.Link,
			"string": activeTheme.Roles.SyntaxString,
		} {
			if rgb := sgrRGB(colour); !strings.Contains(out, rgb) {
				test.Errorf("%s theme: output lacks the %s role colour %s: %q", activeTheme.Identifier, name, rgb, out)
			}
		}
	}
}

// TestOverrideColoursCodeBlocks: a syntax role override reaches rendered
// Markdown code, not just the first theme registered.
func TestOverrideColoursCodeBlocks(test *testing.T) {
	const markdownSource = "```go\nvar s = \"x\"\n```"
	palette := testPalette(test)
	base := theme.FromPalette(palette)
	over := theme.FromPaletteWithOverrides(palette, theme.Roles{SyntaxString: color.RGBA{R: 255, B: 255, A: 255}})
	if out := New(base).Render(markdownSource, 40); strings.Contains(out, "2;255;0;255") {
		test.Fatalf("setup: default theme already uses the override colour: %q", out)
	}
	if out := New(over).Render(markdownSource, 40); !strings.Contains(out, "2;255;0;255") {
		test.Errorf("overridden string colour missing from the rendered code block: %q", out)
	}
}

// TestStyleConfigFollowsPalette builds the Glamour style from two very
// different palettes and checks every colour in it changes.
func TestStyleConfigFollowsPalette(test *testing.T) {
	loud := testPalette(test)
	first, entries := collectColours(styleConfig(theme.New(true))), collectColours(styleConfig(theme.FromPalette(loud)))
	if len(first) < 15 {
		test.Fatalf("only %d colours found in the Glamour style; the walk is broken", len(first))
	}
	for path, callback := range first {
		if second, ok := entries[path]; !ok || callback == second {
			test.Errorf("%s is %q in both palettes (or missing); is it hardcoded?", path, callback)
		}
	}
}

// collectColours returns every non-nil *string field named Color or
// BackgroundColor in value, by path.
func collectColours(value any) map[string]string {
	out := map[string]string{}
	var walk func(reflect.Value, string)
	walk = func(value reflect.Value, path string) {
		if value.Kind() != reflect.Struct {
			return
		}
		for i := range value.NumField() {
			field, name := value.Field(i), path+"."+value.Type().Field(i).Name
			switch {
			case field.Kind() == reflect.Struct:
				walk(field, name)
			case field.Kind() == reflect.Pointer && field.Type().Elem().Kind() == reflect.String && !field.IsNil():
				if variableName := value.Type().Field(i).Name; variableName == "Color" || variableName == "BackgroundColor" {
					out[name] = field.Elem().String()
				}
			}
		}
	}
	walk(reflect.ValueOf(value), "")
	return out
}

func testPalette(test *testing.T) theme.Palette {
	test.Helper()
	var blue16 [16]color.Color
	for i := range blue16 {
		blue16[i] = color.RGBA{R: uint8(255 - i*13), G: uint8(30 + i*11), B: uint8(7 + i*9), A: 255}
	}
	palette, err := theme.FromBase16("loud", theme.Dark, blue16)
	if err != nil {
		test.Fatal(err)
	}
	return palette
}
