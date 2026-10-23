package markdown

import (
	"image/color"
	"strings"

	"charm.land/glamour/v2/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func ptr[Value any](value Value) *Value { return &value }

// styleConfig builds the Glamour style from the theme's roles, so Markdown
// follows the palette like every other pane. Layout (margins, list indents,
// bullets) is Glamour's usual; colours and emphasis are ours. Headings keep
// their "#" markers, drawn in the muted role, with the text in the level's
// colour.
func styleConfig(activeTheme theme.Theme) ansi.StyleConfig {
	roles := activeTheme.Roles
	column := func(color color.Color) *string { return ptr(theme.HexString(color)) }
	text, muted := column(roles.Text), column(roles.Muted)
	accent, link := column(roles.Accent), column(roles.Link)
	bold := ptr(true)

	// The marker is pre-styled (muted) inside the prefix, so it keeps its own
	// colour while the heading text takes the block's.
	heading := func(level int, pointer *string) ansi.StyleBlock {
		marker := activeTheme.HeadingMarker.Render(strings.Repeat("#", level) + " ")
		return ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: marker, Color: pointer, Bold: bold}}
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
			Prefix: " ", Suffix: " ", Color: column(roles.OnAccent), BackgroundColor: accent, Bold: bold,
		}},
		H2: heading(2, accent),
		H3: heading(3, accent),
		H4: heading(4, column(roles.Info)),
		H5: heading(5, column(roles.Info)),
		H6: heading(6, text),

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
			Prefix: "\u00a0", Suffix: "\u00a0", Color: text, BackgroundColor: column(roles.Surface), // non-breaking: no wrap inside a span
		}},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: text},
				Margin:         ptr(uint(2)),
			},
			// The same generated style the diff's token colours come from.
			Theme: activeTheme.ChromaStyleName(),
		},
		DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n🠶 "},
	}
}
