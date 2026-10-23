package markdown

import (
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

var (
	sgr  = regexp.MustCompile(`\x1b\[[0-9;:]*m`)
	osc8 = regexp.MustCompile(`\x1b\]8;[^;\x07\x1b]*;[^\x07\x1b]*\x07`)
)

// TestCharacterReferencesCannotInjectControls checks the property the
// renderer promises: Glamour decodes HTML character references, so the output
// is cleaned and holds no escape sequence beyond SGR and OSC 8.
func TestCharacterReferencesCannotInjectControls(test *testing.T) {
	payloads := map[string]string{
		"text":        "hi &#x1b;]52;c;aGk=&#x07; &#x202e;gnp.exe &rlm; &lrm; &#27;[2J",
		"inline html": "<span>&#x1b;[2J &#x1b;]0;pwned&#7;</span>",
		"code span":   "`&#x1b;]52;c;aGk=&#x07;` and `&#x202e;`",
		"code block":  "```\n&#x1b;]52;c;aGk=&#x07;\n&#x202e;\n```",
		"link text":   "[&#x1b;[2J](http://x/)",
		"heading":     "# &#x1b;]0;t&#x07;",
		"table":       "| a |\n|---|\n| &#x1b;[2J |",
	}
	for name, markdownSource := range payloads {
		test.Run(name, func(test *testing.T) {
			out := New(theme.New(true)).Render(markdownSource, 60)
			rest := sgr.ReplaceAllString(osc8.ReplaceAllString(out, ""), "")
			if strings.ContainsRune(rest, 0x1b) {
				test.Errorf("non-SGR escape in output: %q", rest)
			}
			for _, character := range ansi.Strip(out) {
				if (unicode.IsControl(character) && character != '\n') || unicode.Is(unicode.Bidi_Control, character) {
					test.Errorf("control or bidi rune U+%04X in output %q", character, out)
				}
			}
		})
	}
}
