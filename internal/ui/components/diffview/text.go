package diffview

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// tabWidth is the fixed tab stop used to expand tabs.
const tabWidth = 4

// clusterWidth returns the display width of one grapheme cluster, measured the
// way lipgloss and the terminal renderer measure it.
func clusterWidth(c string) int {
	if len(c) == 1 && c[0] >= 0x20 && c[0] < 0x7f {
		return 1
	}
	return ansi.StringWidth(c)
}

func isUnsafe(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)
}

// displayText makes untrusted text safe and measurable for the terminal: tabs
// expand to tab stops, and control and bidi-control characters (which could
// inject escape sequences or reorder text) become a visible dot. It returns
// the text and its width in cells. Every string from a diff that reaches the
// screen must pass through it.
func displayText(s string) (string, int) {
	if isPlainASCII(s) {
		return s, len(s)
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	col := 0
	for rest := s; rest != ""; {
		var c string
		c, rest, _, _ = uniseg.FirstGraphemeClusterInString(rest, -1)
		if c == "\t" {
			n := tabWidth - col%tabWidth
			b.WriteString(spaces(n))
			col += n
			continue
		}
		if strings.IndexFunc(c, isUnsafe) >= 0 {
			for _, r := range c {
				if isUnsafe(r) {
					b.WriteRune('·')
					col++
				}
			}
			continue
		}
		b.WriteString(c)
		col += clusterWidth(c)
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
// [off, off+width), plus its width. Grapheme clusters are the unit: one cut by
// either edge is replaced by spaces so everything keeps its column.
func window(spans []Span, off, width int) ([]Span, int) {
	end := off + width
	col := 0
	var out []Span
	for _, sp := range spans {
		var text string
		done := false
		if isPlainASCII(sp.Text) {
			n := len(sp.Text)
			if col+n <= off {
				col += n
				continue
			}
			lo, hi := max(off-col, 0), min(n, end-col)
			text = sp.Text[lo:hi]
			if hi < n {
				col, done = end, true
			} else {
				col += n
			}
		} else {
			text, col, done = windowSlow(sp.Text, col, off, end)
		}
		if text != "" {
			out = append(out, Span{Text: text, Class: sp.Class})
		}
		if done || col >= end {
			break
		}
	}
	used := min(max(col-off, 0), width)
	return out, used
}

func windowSlow(s string, col, off, end int) (string, int, bool) {
	var b strings.Builder
	for rest := s; rest != ""; {
		var c string
		c, rest, _, _ = uniseg.FirstGraphemeClusterInString(rest, -1)
		w := clusterWidth(c)
		switch {
		case w == 0:
			if col > off { // zero-width cluster: keep only after a visible one
				b.WriteString(c)
			}
		case col+w <= off:
			col += w
		case col < off: // straddles the left edge
			b.WriteString(spaces(col + w - off))
			col += w
		case col+w > end: // does not fit on the right
			b.WriteString(spaces(end - col))
			return b.String(), end, true
		default:
			b.WriteString(c)
			col += w
		}
	}
	return b.String(), col, false
}
