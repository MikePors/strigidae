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

func newStyleSet(theme theme.Theme) styleSet {
	var style styleSet
	cursorBackground := theme.DiffCursor.GetBackground()
	bases := [numberKinds]lipgloss.Style{theme.DiffContext, theme.DiffAdd, theme.DiffDelete}
	gutters := [numberKinds]lipgloss.Style{theme.DiffGutter, theme.DiffGutterAdd, theme.DiffGutterDelete}
	var foreground [numberClasses + 1]lipgloss.Style
	for class := range Class(numberClasses) {
		foreground[class] = theme.Syntax.For(class)
	}
	foreground[ClassPlain] = lipgloss.NewStyle()
	foreground[classNoNewline] = theme.DiffNoNewline
	for current := range 2 {
		for k := range numberKinds {
			base, gutter := bases[k], gutters[k]
			if current == 1 {
				base, gutter = base.Background(cursorBackground), gutter.Background(cursorBackground)
			}
			view := variant{fill: base, gutter: gutter}
			view.thread[0] = theme.Muted.Background(gutter.GetBackground())
			view.thread[1] = theme.ThreadMarker.Background(gutter.GetBackground())
			for count := range view.code {
				view.code[count] = foreground[count].Background(base.GetBackground())
			}
			style.variants[current][k] = view
		}
	}
	style.hunk[0] = theme.DiffHunk
	style.hunk[1] = theme.DiffHunkCursor
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
		return model.renderHunk(model.file.Hunks[row.hunk].Header(), current)
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
	view := &model.styles.variants[current][kind]

	gutter := model.gutterText(row.line, row.baseIndex, mark, view)
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
		builder.WriteString(view.code[span.Class].Render(span.Text))
	}
	if pad := codeWidth - used; pad > 0 {
		builder.WriteString(view.fill.Render(spaces(pad)))
	}
	return builder.String()
}

// gutterText is the styled fixed part of a diff row: cursor mark, line
// numbers, thread marker, change marker.
func (model *Model) gutterText(line *diff.Line, baseIndex int, mark string, view *variant) string {
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
	titleStyle := view.gutter
	if glyph != " " {
		titleStyle = view.thread[0]
		if open {
			titleStyle = view.thread[1]
		}
	}
	return view.gutter.Render(mark+numberCell(line.Old, model.numberWidth)+" "+numberCell(line.New, model.numberWidth)+" ") +
		titleStyle.Render(glyph) + view.gutter.Render(marker)
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
