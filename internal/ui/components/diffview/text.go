package diffview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// tabWidth is the fixed tab stop used to expand tabs.
const tabWidth = 4

// runeWidth returns the display width of r in terminal cells.
func runeWidth(r rune) int {
	if r >= 0x20 && r < 0x7f {
		return 1
	}
	return ansi.StringWidth(string(r))
}

// displayText makes code safe and measurable for the terminal: tabs expand to
// tab stops and control characters (which could inject escape sequences) become
// a visible dot. It returns the text and its width in cells.
func displayText(s string) (string, int) {
	if isPlainASCII(s) {
		return s, len(s)
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	col := 0
	for _, r := range s {
		switch {
		case r == '\t':
			n := tabWidth - col%tabWidth
			b.WriteString(spaces(n))
			col += n
		case r < 0x20, r == 0x7f, r >= 0x80 && r < 0xa0:
			b.WriteRune('·')
			col++
		default:
			b.WriteRune(r)
			col += runeWidth(r)
		}
	}
	return b.String(), col
}

func isPlainASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] >= 0x7f {
			return false
		}
	}
	return true
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// window returns the part of spans that falls in display columns
// [off, off+width), plus its width. A wide character cut by either edge is
// replaced by spaces so everything keeps its column.
func window(spans []Span, off, width int) ([]Span, int) {
	end := off + width
	col := 0
	var out []Span
	for _, sp := range spans {
		var b strings.Builder
		done := false
		for _, r := range sp.Text {
			w := runeWidth(r)
			switch {
			case w == 0:
				if col > off { // combining mark: keep only if its base is visible
					b.WriteRune(r)
				}
			case col+w <= off:
				col += w
			case col < off: // straddles the left edge
				b.WriteString(spaces(col + w - off))
				col += w
			case col+w > end: // does not fit on the right
				b.WriteString(spaces(end - col))
				col = end
				done = true
			default:
				b.WriteRune(r)
				col += w
			}
			if done {
				break
			}
		}
		if b.Len() > 0 {
			out = append(out, Span{Text: b.String(), Class: sp.Class})
		}
		if done || col >= end {
			break
		}
	}
	used := min(max(col-off, 0), width)
	return out, used
}
