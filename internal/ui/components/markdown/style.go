package markdown

import (
	"image/color"

	"charm.land/glamour/v2/ansi"

	"github.com/MikePors/strigidae/internal/ui/theme"
)

func ptr[T any](v T) *T { return &v }

// styleConfig builds the Glamour style from the theme's roles, so Markdown
// follows the palette like every other pane. Layout (margins, list indents,
// bullets) is Glamour's usual; colours and emphasis are ours. Headings carry
// no "#" markers: level shows through colour and weight instead.
func styleConfig(th theme.Theme) ansi.StyleConfig {
	r := th.Roles
	col := func(c color.Color) *string { return ptr(theme.HexString(c)) }
	text, muted := col(r.Text), col(r.Muted)
	accent, link := col(r.Accent), col(r.Link)
	bold := ptr(true)

	heading := func(c *string) ansi.StyleBlock {
		return ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: c, Bold: bold}}
	}
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockPrefix: "\n", BlockSuffix: "\n", Color: text},
			Margin:         ptr(uint(2)),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: muted},
			Indent:         ptr(uint(1)),
			IndentToken:    ptr("│ "),
		},
		List: ansi.StyleList{LevelIndent: 2},

		Heading: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n", Color: accent, Bold: bold}},
		H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			Prefix: " ", Suffix: " ", Color: col(r.OnAccent), BackgroundColor: accent, Bold: bold,
		}},
		H2: heading(accent),
		H3: heading(accent),
		H4: heading(col(r.Info)),
		H5: heading(col(r.Info)),
		H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: muted, Bold: bold}},

		Strikethrough:  ansi.StylePrimitive{CrossedOut: bold},
		Emph:           ansi.StylePrimitive{Italic: bold},
		Strong:         ansi.StylePrimitive{Bold: bold},
		HorizontalRule: ansi.StylePrimitive{Color: muted, Format: "\n--------\n"},

		Item:        ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration: ansi.StylePrimitive{BlockPrefix: ". "},
		Task:        ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},

		Link:      ansi.StylePrimitive{Color: link, Underline: bold},
		LinkText:  ansi.StylePrimitive{Color: link, Bold: bold},
		Image:     ansi.StylePrimitive{Color: accent, Underline: bold},
		ImageText: ansi.StylePrimitive{Color: muted, Format: "Image: {{.text}} →"},

		// Inline code is the text colour on a muted surface, not a hue.
		Code: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			Prefix: "\u00a0", Suffix: "\u00a0", Color: text, BackgroundColor: col(r.Surface), // non-breaking: no wrap inside a span
		}},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: text},
				Margin:         ptr(uint(2)),
			},
			// The same generated style the diff's token colours come from.
			Theme: th.ChromaStyleName(),
		},
		DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n🠶 "},
	}
}
