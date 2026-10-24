package diffview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// classNoNewline styles the text of "\ No newline" marker rows.
const classNoNewline = Class(numberClasses)

// Line-kind indexes into the style set.
const (
	kindContext = iota
	kindAdd
	kindDelete
	numberKinds
)

// variant is the set of styles for one (kind, cursor) combination.
type variant struct {
	code   [numberClasses + 1]lipgloss.Style
	fill   lipgloss.Style // pads the code area to full width
	gutter lipgloss.Style
	thread [2]lipgloss.Style // gutter marker: [0] quiet thread, [1] open thread
}

// styleSet precomputes every style combination so rendering never builds
// styles.
type styleSet struct {
	variants [2][numberKinds]variant // [cursor][kind]
	hunk     [2]lipgloss.Style
}

func newStyleSet(activeTheme theme.Theme) styleSet {
	var style styleSet
	cursorBackground := activeTheme.DiffCursor.GetBackground()
	bases := [numberKinds]lipgloss.Style{activeTheme.DiffContext, activeTheme.DiffAdd, activeTheme.DiffDelete}
	gutters := [numberKinds]lipgloss.Style{activeTheme.DiffGutter, activeTheme.DiffGutterAdd, activeTheme.DiffGutterDelete}
	var foreground [numberClasses + 1]lipgloss.Style
	for class := range Class(numberClasses) {
		foreground[class] = activeTheme.Syntax.For(class)
	}
	foreground[ClassPlain] = lipgloss.NewStyle()
	foreground[classNoNewline] = activeTheme.DiffNoNewline
	for current := range 2 {
		for k := range numberKinds {
			base, gutter := bases[k], gutters[k]
			if current == 1 {
				base, gutter = base.Background(cursorBackground), gutter.Background(cursorBackground)
			}
			variantStyles := variant{fill: base, gutter: gutter}
			variantStyles.thread[0] = activeTheme.Muted.Background(gutter.GetBackground())
			variantStyles.thread[1] = activeTheme.ThreadMarker.Background(gutter.GetBackground())
			for i := range variantStyles.code {
				variantStyles.code[i] = foreground[i].Background(base.GetBackground())
			}
			style.variants[current][k] = variantStyles
		}
	}
	style.hunk[0] = activeTheme.DiffHunk
	style.hunk[1] = activeTheme.DiffHunkCursor
	return style
}

func (model *Model) renderRow(i int) string {
	row := model.rows[i]
	current := 0
	if i == model.window.Cursor() {
		current = 1
	}
	if row.threadRow != nil {
		return model.renderThreadRow(i, row)
	}
	if row.message {
		return model.renderNoticeRow(i, row)
	}
	if row.line == nil {
		header := model.file.Hunks[row.hunk].Header()
		if model.hunkReviewed != nil && model.hunkReviewed(model.file, row.hunk) {
			header = markerReviewed + " " + header
		}
		return model.renderHunk(header, current)
	}
	var kind int
	switch row.line.Kind {
	case diff.Add:
		kind = kindAdd
	case diff.Delete:
		kind = kindDelete
	}
	mark := " "
	switch {
	case current == 1:
		mark = ">"
	case model.selected(row.baseIndex):
		mark, current = markSelected, 1 // the selection takes the cursor tint
	}
	variantStyles := &model.styles.variants[current][kind]

	gutter := model.gutterText(row.line, row.baseIndex, mark, variantStyles)
	codeWidth := model.codeWidth()
	if codeWidth == 0 {
		return ansi.Truncate(gutter, model.width, "")
	}

	spans := model.spansFor(i)
	if row.line.Kind == diff.NoNewline {
		spans = []Span{{Text: row.text, Class: classNoNewline}}
	}
	visible, used := window(spans, model.window.HorizontalOffset(), codeWidth)

	var builder strings.Builder
	builder.WriteString(gutter)
	for _, span := range visible {
		builder.WriteString(variantStyles.code[span.Class].Render(span.Text))
	}
	if pad := codeWidth - used; pad > 0 {
		builder.WriteString(variantStyles.fill.Render(spaces(pad)))
	}
	return builder.String()
}

// gutterText is the styled fixed part of a diff row: cursor mark, line
// numbers, thread marker, change marker.
func (model *Model) gutterText(line *diff.Line, baseIndex int, mark string, variantStyles *variant) string {
	marker := " "
	switch line.Kind {
	case diff.Add:
		marker = "+"
	case diff.Delete:
		marker = "-"
	case diff.NoNewline:
		marker = `\`
	}
	glyph, open := model.threadGlyph(baseIndex)
	titleStyle := variantStyles.gutter
	if glyph != " " {
		titleStyle = variantStyles.thread[0]
		if open {
			titleStyle = variantStyles.thread[1]
		}
	}
	return variantStyles.gutter.Render(mark+numberCell(line.Old, model.numberWidth)+" "+numberCell(line.New, model.numberWidth)+" ") +
		titleStyle.Render(glyph) + variantStyles.gutter.Render(marker)
}

func numberCell(count, width int) string {
	text := ""
	if count > 0 {
		text = strconv.Itoa(count)
	}
	return spaces(width-len(text)) + text
}

func (model *Model) renderHunk(header string, current int) string {
	mark := " "
	if current == 1 {
		mark = ">"
	}
	header, _ = displayText(header) // the section text is copied from file content
	text := ansi.Truncate(mark+header, model.width, "")
	text += spaces(model.width - ansi.StringWidth(text))
	return model.styles.hunk[current].Render(text)
}

// spansFor returns the highlighted spans of row i, highlighting on first use.
func (model *Model) spansFor(i int) []Span {
	baseIndex := model.rows[i].baseIndex
	if span, ok := model.cache[baseIndex]; ok {
		return span
	}
	span := model.highlight(model.file.Path(), model.rows[i].text)
	model.cache[baseIndex] = span
	return span
}
