package scheme

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func readFixture(test *testing.T, name string) string {
	test.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		test.Fatal(err)
	}
	return string(data)
}

func TestParseFixtures(test *testing.T) {
	tests := []struct {
		file    string
		name    string
		variant theme.Variant
		text    string // base05, hex
		last    string // base17, hex; empty for Base16, which derives it
	}{
		{"base16-current.yaml", "Solarized Light", theme.Light, "#586e75", ""},
		{"base16-flat.yaml", "Solarized Light", theme.Light, "#586e75", ""},
		{"base24-current.yaml", "Tokyo Night Dark", theme.Dark, "#a9b1d6", "#bb9af7"},
		{"base24-flat.yaml", "Tokyo Night Dark", theme.Dark, "#a9b1d6", "#bb9af7"},
	}
	for _, testCase := range tests {
		test.Run(testCase.file, func(test *testing.T) {
			scheme, err := Parse(testCase.file, []byte(readFixture(test, testCase.file)))
			if err != nil {
				test.Fatal(err)
			}
			if scheme.Name != testCase.name {
				test.Errorf("name = %q, want %q", scheme.Name, testCase.name)
			}
			if scheme.Palette.Variant != testCase.variant {
				test.Errorf("variant = %v, want %v", scheme.Palette.Variant, testCase.variant)
			}
			if got := theme.HexString(scheme.Palette.Slot(5)); got != testCase.text {
				test.Errorf("base05 = %s, want %s", got, testCase.text)
			}
			if got := theme.HexString(scheme.Palette.Slot(23)); testCase.last != "" && got != testCase.last {
				test.Errorf("base17 = %s, want %s", got, testCase.last)
			}
		})
	}
}

// The flat and current layouts of one scheme build the same theme.
func TestLayoutsAgree(test *testing.T) {
	for _, pair := range [][2]string{{"base16-current.yaml", "base16-flat.yaml"}, {"base24-current.yaml", "base24-flat.yaml"}} {
		current, err := Parse(pair[0], []byte(readFixture(test, pair[0])))
		if err != nil {
			test.Fatal(err)
		}
		flat, err := Parse(pair[1], []byte(readFixture(test, pair[1])))
		if err != nil {
			test.Fatal(err)
		}
		if current.Theme().Identifier != flat.Theme().Identifier {
			test.Errorf("%s and %s build different themes", pair[0], pair[1])
		}
	}
}

// grey16 is a Base16 scheme in the current layout whose base00 is background.
func grey16(background string) string {
	var builder strings.Builder
	builder.WriteString("palette:\n")
	for index := range 16 {
		colour := "808080"
		if index == 0 {
			colour = background
		}
		fmt.Fprintf(&builder, "  base%02X: \"%s\"\n", index, colour)
	}
	return builder.String()
}

func TestParseVariant(test *testing.T) {
	tests := []struct {
		name string
		text string
		want theme.Variant
	}{
		{"inferred dark", grey16("101010"), theme.Dark},
		{"inferred light", grey16("f0f0f0"), theme.Light},
		{"declared light wins", "variant: light\n" + grey16("101010"), theme.Light},
		{"declared dark wins", "variant: dark\n" + grey16("f0f0f0"), theme.Dark},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			scheme, err := Parse("variant.yaml", []byte(testCase.text))
			if err != nil {
				test.Fatal(err)
			}
			if scheme.Palette.Variant != testCase.want {
				test.Errorf("variant = %v, want %v", scheme.Palette.Variant, testCase.want)
			}
			if scheme.Name != "variant" {
				test.Errorf("name = %q, want the file name", scheme.Name)
			}
		})
	}
}

func TestParseRoles(test *testing.T) {
	text := grey16("101010") + "roles:\n  muted: \"#112233\"\n  border: 445566\n  added-tint: \"#010203\"\n"
	scheme, err := Parse("roles.yaml", []byte(text))
	if err != nil {
		test.Fatal(err)
	}
	built := scheme.Theme()
	for name, got := range map[string]string{
		"#112233": theme.HexString(built.Roles.Muted),
		"#445566": theme.HexString(built.Roles.Border),
		"#010203": theme.HexString(built.Roles.AddedTint),
	} {
		if got != name {
			test.Errorf("got %s, want the exact override %s", got, name)
		}
	}
	if !built.Chosen {
		test.Error("a scheme's theme is not marked Chosen")
	}
}

func TestParseSyntaxWarns(test *testing.T) {
	scheme, err := Parse("syntax.yaml", []byte(grey16("101010")+"syntax: tokyonight-night\n"))
	if err != nil {
		test.Fatal(err)
	}
	if len(scheme.Warnings) != 1 || !strings.Contains(scheme.Warnings[0], "tokyonight-night") {
		test.Errorf("warnings = %q, want one naming the syntax style", scheme.Warnings)
	}
}

func TestParseErrors(test *testing.T) {
	base16 := readFixture(test, "base16-current.yaml")
	base24 := readFixture(test, "base24-current.yaml")
	flat := readFixture(test, "base16-flat.yaml")
	tests := []struct {
		name string
		text string
		want []string // every one must appear in the error
	}{
		{"empty", "", []string{"t.yaml", "empty"}},
		{"not yaml", "a: [", []string{"t.yaml", "invalid YAML"}},
		{"not a mapping", "- a\n", []string{"t.yaml", "mapping"}},
		{"unknown key", base16 + "colour: red\n", []string{"t.yaml:", `unknown key "colour"`, "valid keys"}},
		{"flat key in current layout", base16 + "scheme: x\n", []string{`unknown key "scheme"`}},
		{"current key in flat layout", flat + "system: base16\n", []string{`unknown key "system"`}},
		{"unknown palette key", strings.Replace(base16, "base0F:", "base1F:", 1), []string{`unknown key "base1F"`, "the palette"}},
		{"duplicate key", base16 + "name: again\n", []string{`duplicate key "name"`}},
		{"bad system", strings.Replace(base16, `"base16"`, `"base32"`, 1), []string{`system "base32"`}},
		{"missing slot", strings.Replace(base16, "  base05: \"#586e75\"\n", "", 1), []string{"missing base05"}},
		{"base16 scheme with base10", strings.Replace(base16, "  base0F:", "  base10: \"#000000\"\n  base0F:", 1), []string{"has base10", "base16"}},
		{"base24 scheme missing base17", strings.Replace(base24, "  base17: \"#bb9af7\" # Bright magenta\n", "", 1), []string{"missing base17"}},
		{"malformed colour", strings.Replace(base16, "#586e75", "#58z", 1), []string{"base05", `invalid colour "#58z"`}},
		{"colour without quotes is a comment", strings.Replace(base16, `"#586e75"`, "#586e75", 1), []string{"base05", "has no value"}},
		{"bad variant", strings.Replace(base16, `variant: "light"`, "variant: grey", 1), []string{`variant "grey"`}},
		{"unknown role", base16 + "roles:\n  warmth: \"#ffffff\"\n", []string{`unknown role "warmth"`, "valid roles:", "muted", "border", "added-tint"}},
		{"malformed role colour", base16 + "roles:\n  muted: blue\n", []string{"roles: muted", `invalid colour "blue"`}},
		{"two documents", base16 + "---\na: b\n", []string{"single YAML document"}},
		{"too large", strings.Repeat("#", MaximumFileSize+1), []string{"too large"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			_, err := Parse("t.yaml", []byte(testCase.text))
			if err == nil {
				test.Fatal("no error")
			}
			for _, fragment := range testCase.want {
				if !strings.Contains(err.Error(), fragment) {
					test.Errorf("error %q lacks %q", err, fragment)
				}
			}
		})
	}
}
