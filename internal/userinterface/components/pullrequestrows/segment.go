package pullrequestrows

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const ellipsis = "…"

// segment is a run of plain text and the theme style it is painted with. The
// text never holds escape codes: widths are measured on it, and the style is
// applied only when the row is painted.
type segment struct {
	text  string
	style lipgloss.Style
}

// cell is the content of one column: one or more segments, left to right.
// A row is a cell too: its columns and the gaps between them.
type cell []segment

// styled returns a cell of one segment.
func styled(text string, style lipgloss.Style) cell {
	return cell{{text: text, style: style}}
}

// plain returns the text of every segment, without styles.
func (content cell) plain() string {
	var text strings.Builder
	for _, part := range content {
		text.WriteString(part.text)
	}
	return text.String()
}

// truncate cuts the cell to width display columns, ending it with "…" when
// something was cut. The ellipsis takes the style of the segment it cuts into.
func (content cell) truncate(width int) cell {
	text := content.plain()
	cut := ansi.Truncate(text, width, ellipsis)
	if cut == text {
		return content
	}
	kept, tail := cut, ""
	if trimmed := strings.TrimSuffix(cut, ellipsis); trimmed != cut && strings.HasPrefix(text, trimmed) {
		kept, tail = trimmed, ellipsis
	}
	remaining := len(kept)
	var out cell
	for _, part := range content {
		if remaining > 0 && remaining >= len(part.text) {
			out = append(out, part)
			remaining -= len(part.text)
			continue
		}
		if cutText := part.text[:remaining] + tail; cutText != "" {
			out = append(out, segment{text: cutText, style: part.style})
		}
		return out
	}
	if tail != "" && len(out) > 0 {
		out[len(out)-1].text += tail
	}
	return out
}

// padRight adds spaces after the cell, in the style of its last segment, until
// it is width display columns wide. A cell already that wide is returned as is.
func (content cell) padRight(width int) cell {
	count := width - ansi.StringWidth(content.plain())
	if count <= 0 || len(content) == 0 {
		return content
	}
	out := append(cell(nil), content...)
	out[len(out)-1].text += strings.Repeat(" ", count)
	return out
}

// padLeft adds spaces before the cell, in the style of its first segment.
func (content cell) padLeft(width int) cell {
	count := width - ansi.StringWidth(content.plain())
	if count <= 0 || len(content) == 0 {
		return content
	}
	out := append(cell(nil), content...)
	out[0].text = strings.Repeat(" ", count) + out[0].text
	return out
}

// paint applies each segment's own style.
func (content cell) paint() string {
	var out strings.Builder
	for _, part := range content {
		if part.text != "" {
			out.WriteString(part.style.Render(part.text))
		}
	}
	return out.String()
}

// paintAll paints the whole cell in one style, ignoring the segments' own: the
// selected row and the header.
func (content cell) paintAll(style lipgloss.Style) string {
	return style.Render(content.plain())
}
