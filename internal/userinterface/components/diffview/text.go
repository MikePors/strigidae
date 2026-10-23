package diffview

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// tabWidth is the fixed tab stop used to expand tabs.
const tabWidth = 4

// clusterWidth returns the display width of one grapheme cluster, measured the
// way lipgloss and the terminal renderer measure it.
func clusterWidth(text string) int {
	if len(text) == 1 && text[0] >= 0x20 && text[0] < 0x7f {
		return 1
	}
	return ansi.StringWidth(text)
}

func isUnsafe(character rune) bool {
	return unicode.IsControl(character) || unicode.Is(unicode.Bidi_Control, character)
}

// displayText makes untrusted text safe and measurable for the terminal: tabs
// expand to tab stops, and control and bidi-control characters (which could
// inject escape sequences or reorder text) become a visible dot. It returns
// the text and its width in cells. Every string from a diff that reaches the
// screen must pass through it. It applies the same unsafe-rune rule as
// internal/ui/termtext, but also expands tabs and measures grapheme widths.
func displayText(text string) (string, int) {
	if isPlainASCII(text) {
		return text, len(text)
	}
	if !utf8.ValidString(text) {
		text = replaceInvalid(text)
	}
	var builder strings.Builder
	builder.Grow(len(text) + 8)
	column := 0
	for rest := text; rest != ""; {
		var text string
		text, rest, _, _ = uniseg.FirstGraphemeClusterInString(rest, -1)
		if text == "\t" {
			count := tabWidth - column%tabWidth
			builder.WriteString(spaces(count))
			column += count
			continue
		}
		if strings.IndexFunc(text, isUnsafe) >= 0 {
			for _, character := range text {
				if isUnsafe(character) {
					builder.WriteRune('·')
					column++
				}
			}
			continue
		}
		builder.WriteString(text)
		column += clusterWidth(text)
	}
	return builder.String(), column
}

// replaceInvalid turns every invalid UTF-8 byte into a visible dot. Without
// this a stray byte such as 0x9B (an 8-bit CSI) would reach the terminal.
func replaceInvalid(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	for i := 0; i < len(text); {
		character, size := utf8.DecodeRuneInString(text[i:])
		if character == utf8.RuneError && size == 1 {
			builder.WriteRune('·')
		} else {
			builder.WriteString(text[i : i+size])
		}
		i += size
	}
	return builder.String()
}

func isPlainASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] < 0x20 || text[i] >= 0x7f {
			return false
		}
	}
	return true
}

func spaces(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.Repeat(" ", count)
}

// window returns the part of spans that falls in display columns
// [off, off+width), plus its width. Grapheme clusters are the unit: one cut by
// either edge is replaced by spaces so everything keeps its column.
func window(spans []Span, off, width int) ([]Span, int) {
	end := off + width
	column := 0
	var out []Span
	for _, span := range spans {
		var text string
		done := false
		if isPlainASCII(span.Text) {
			count := len(span.Text)
			if column+count <= off {
				column += count
				continue
			}
			low, high := max(off-column, 0), min(count, end-column)
			text = span.Text[low:high]
			if high < count {
				column, done = end, true
			} else {
				column += count
			}
		} else {
			text, column, done = windowSlow(span.Text, column, off, end)
		}
		if text != "" {
			out = append(out, Span{Text: text, Class: span.Class})
		}
		if done || column >= end {
			break
		}
	}
	used := min(max(column-off, 0), width)
	return out, used
}

func windowSlow(text string, column, off, end int) (string, int, bool) {
	var builder strings.Builder
	for rest := text; rest != ""; {
		var text string
		text, rest, _, _ = uniseg.FirstGraphemeClusterInString(rest, -1)
		width := clusterWidth(text)
		switch {
		case width == 0:
			if column > off { // zero-width cluster: keep only after a visible one
				builder.WriteString(text)
			}
		case column+width <= off:
			column += width
		case column < off: // straddles the left edge
			builder.WriteString(spaces(column + width - off))
			column += width
		case column+width > end: // does not fit on the right
			builder.WriteString(spaces(end - column))
			return builder.String(), end, true
		default:
			builder.WriteString(text)
			column += width
		}
	}
	return builder.String(), column, false
}
