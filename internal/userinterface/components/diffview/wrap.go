package diffview

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// minimumWrapWidth is the narrowest code area that wraps: the widest
// grapheme cluster a terminal draws is two cells.
const minimumWrapWidth = 2

// markerContinued stands in the new-line-number column of a wrapped line's
// continuation rows.
const markerContinued = "↪"

// WithWrap makes long lines continue on the next screen rows instead of
// running off the pane's right edge.
func WithWrap(wrap bool) Option { return func(model *Model) { model.wrap = wrap } }

// SetWrap turns wrapping on or off. The cursor stays on its line.
func (model *Model) SetWrap(wrap bool) {
	if wrap == model.wrap {
		return
	}
	model.wrap = wrap
	model.sync()
}

// Wrap reports whether long lines wrap.
func (model *Model) Wrap() bool { return model.wrap }

// rowHeight returns how many screen rows row i takes: more than one only for
// a wrapped line longer than the code area.
func (model *Model) rowHeight(i int) int {
	if !model.wrap || i >= len(model.rows) || !model.rows[i].isCode() {
		return 1
	}
	return len(model.rowStarts(i))
}

// rowStarts returns the column each screen row of code row i starts at, the
// first being 0. It is computed when first asked for and kept until the code
// width changes, so only lines near the view are measured.
func (model *Model) rowStarts(i int) []int {
	width := model.codeWidth()
	if model.wrapStarts == nil || model.wrapWidth != width {
		model.wrapStarts, model.wrapWidth = map[int][]int{}, width
	}
	baseIndex := model.rows[i].baseIndex
	if starts, ok := model.wrapStarts[baseIndex]; ok {
		return starts
	}
	starts := wrapColumns(model.rows[i].text, width)
	model.wrapStarts[baseIndex] = starts
	return starts
}

// wrapColumns splits display text into screen rows of at most width cells
// and returns the column each row starts at. Rows break between grapheme
// clusters, so a wide character moves whole to the next row. Below
// minimumWrapWidth a wide character could not fit on any row, so the text
// stays one row, cut off at the edge as without wrapping.
func wrapColumns(text string, width int) []int {
	starts := []int{0}
	if width < minimumWrapWidth {
		return starts
	}
	if isPlainASCII(text) {
		for start := width; start < len(text); start += width {
			starts = append(starts, start)
		}
		return starts
	}
	column, rowStart := 0, 0
	for rest := text; rest != ""; {
		var cluster string
		cluster, rest, _, _ = uniseg.FirstGraphemeClusterInString(rest, -1)
		clusterWidth := clusterWidth(cluster)
		if column+clusterWidth-rowStart > width && column > rowStart {
			starts = append(starts, column)
			rowStart = column
		}
		column += clusterWidth
	}
	return starts
}

// renderWrapped draws the first limit screen rows of code row i (the rows
// still free on screen): the first with the line's gutter, the rest with
// blank numbers and markerContinued. Every row keeps the line's tint, and the
// spans are highlighted once for the whole line. Stopping at limit keeps a
// frame's cost to the screen's size however long the line is.
func (model *Model) renderWrapped(i, limit int) []string {
	row := model.rows[i]
	variantStyles, mark := model.rowStyle(i)
	gutter := model.gutterText(row.line, row.baseIndex, mark, variantStyles)
	codeWidth := model.codeWidth()
	if codeWidth == 0 {
		return []string{ansi.Truncate(gutter, model.width, "")}
	}
	continued := variantStyles.gutter.Render(" " + spaces(model.numberWidth) + " " +
		spaces(model.numberWidth-1) + markerContinued + "   ")
	spans := model.codeSpans(i)
	starts := model.rowStarts(i)
	lines := make([]string, min(len(starts), max(limit, 0)))
	for k := range lines {
		start, width := starts[k], codeWidth
		if k+1 < len(starts) {
			width = starts[k+1] - start
		}
		model.wrappedRows++
		prefix := continued
		if k == 0 {
			prefix = gutter
		}
		lines[k] = prefix + model.codeText(spans, start, width, codeWidth, variantStyles)
	}
	return lines
}
