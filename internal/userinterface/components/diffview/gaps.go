package diffview

import (
	"strconv"

	"github.com/MikePors/strigidae/internal/diff"
)

// gapChunk is how many hidden lines one expansion shows.
const gapChunk = 20

// gapText starts the text of a gap row, after the line-number columns.
const gapText = "⋯ "

// gapRow marks a base row as a gap: the unchanged lines the diff leaves out
// before a hunk (index is that hunk's index), or after the last hunk (index is
// len(File.Hunks)). Such a row also has message set, so everything that skips
// notices skips gaps: they are neither a hunk header nor a diff line.
type gapRow struct {
	index int
	// hidden is how many lines the gap still hides, or -1 while that is not
	// known: the lines below the last hunk are counted only once the file's
	// text is.
	hidden int
}

// appendGapRow adds the row of gap index, unless nothing is left hidden in it.
func (model *Model) appendGapRow(index int) {
	if !canExpand(model.file) {
		return
	}
	if _, _, hidden := model.gapState(index); hidden != 0 {
		model.appendRow(row{hunk: gapHunk(index), message: true, gap: &gapRow{index: index, hidden: hidden}})
	}
}

// appendRevealed adds the lines of gap index shown so far, inside the block of
// the hunk they continue. A gap at the top of the file shows the lines nearest
// the hunk below it, any other the ones nearest the hunk above; either way the
// gap row stays at the far end, so the lines a reader has seen never move.
func (model *Model) appendRevealed(index int, maximumNumber *int) {
	if !canExpand(model.file) {
		return
	}
	span, shown, _ := model.gapState(index)
	from := span.first
	if index == 0 {
		from = span.last - shown + 1
	}
	for number := from; number < from+shown; number++ {
		model.revealedLines = append(model.revealedLines, diff.Line{Kind: diff.Context, Old: number + span.offset, New: number, Text: model.fileLines[number-1]})
		line := &model.revealedLines[len(model.revealedLines)-1]
		text, width := displayText(line.Text)
		model.maximumWidth = max(model.maximumWidth, width)
		*maximumNumber = max(*maximumNumber, line.Old, line.New)
		model.appendRow(row{hunk: gapHunk(index), line: line, text: text, revealed: true})
	}
}

// GapIndex returns which gap the cursor is on (the index of the hunk below it,
// the number of hunks for the one after the last), or -1 when it is on none.
func (model *Model) GapIndex() int {
	if gap := model.cursorGap(); gap != nil {
		return gap.index
	}
	return -1
}

// OnGap reports whether the cursor is on the row of a gap, the row that stands
// for unchanged lines the diff leaves out.
func (model *Model) OnGap() bool { return model.cursorGap() != nil }

func (model *Model) cursorGap() *gapRow {
	if model.window.Cursor() >= len(model.rows) {
		return nil
	}
	return model.rows[model.window.Cursor()].gap
}

// gapRowIndex returns the row of gap index, or -1 when it has none.
func (model *Model) gapRowIndex(index int) int {
	for i, row := range model.rows {
		if row.gap != nil && row.gap.index == index {
			return i
		}
	}
	return -1
}

// hiddenText is what a gap row says about the lines it hides.
func hiddenText(hidden int) string {
	switch hidden {
	case -1:
		return "more lines"
	case 1:
		return "1 hidden line"
	}
	return strconv.Itoa(hidden) + " hidden lines"
}

// renderGap draws the row of gap row, its text under the code column.
func (model *Model) renderGap(i int, row row, current int) string {
	mark := " "
	if current == 1 {
		mark = ">"
	}
	text := spaces(model.lineNumbersWidth()) + gapText + hiddenText(row.gap.hidden)
	return model.markedRow(i, mark, text, model.styles.placeholder[current], model.styles.placeholderRelative[current])
}
