package diffview

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

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

// gapSpan is where the lines of one gap are in the new side of the file.
type gapSpan struct {
	first, last int // line numbers; empty when last < first
	offset      int // a line's old number is its new number plus offset
	known       bool
}

func (span gapSpan) size() int { return max(span.last-span.first+1, 0) }

// The first and last line a hunk covers on each side. A side with no lines in
// the hunk (a pure insertion or deletion) sits after the line its Start names.
func newBegin(hunk diff.Hunk) int {
	if hunk.NewLines == 0 {
		return hunk.NewStart + 1
	}
	return hunk.NewStart
}

func newEnd(hunk diff.Hunk) int {
	if hunk.NewLines == 0 {
		return hunk.NewStart
	}
	return hunk.NewStart + hunk.NewLines - 1
}

func oldBegin(hunk diff.Hunk) int {
	if hunk.OldLines == 0 {
		return hunk.OldStart + 1
	}
	return hunk.OldStart
}

func oldEnd(hunk diff.Hunk) int {
	if hunk.OldLines == 0 {
		return hunk.OldStart
	}
	return hunk.OldStart + hunk.OldLines - 1
}

// canExpand reports whether the shown file has gaps to show: it has hunks and
// exists on both sides, so the lines between them are on its new side. An
// added or deleted file's diff holds the whole file.
func canExpand(file *diff.File) bool {
	if file == nil || file.Binary || len(file.Hunks) == 0 {
		return false
	}
	return file.Status == diff.Modified || file.Status == diff.Renamed || file.Status == diff.Copied
}

// gapSpan returns where gap index is. A gap between hunks is bounded by them;
// the one below the last hunk ends at the end of the file, known only once the
// text is.
func (model *Model) gapSpan(index int) gapSpan {
	hunks := model.file.Hunks
	if index < len(hunks) {
		next := hunks[index]
		first := 1
		if index > 0 {
			first = newEnd(hunks[index-1]) + 1
		}
		last := newBegin(next) - 1
		if model.fileLines != nil {
			last = min(last, len(model.fileLines)) // text that disagrees with the diff
		}
		return gapSpan{first: first, last: last, offset: oldBegin(next) - newBegin(next), known: true}
	}
	previous := hunks[len(hunks)-1]
	span := gapSpan{first: newEnd(previous) + 1, offset: oldEnd(previous) - newEnd(previous)}
	if model.fileLines != nil {
		span.last, span.known = len(model.fileLines), true
	}
	return span
}

// gapState says how much of gap index is shown and how much still hidden
// (-1 when that is not known yet). A file with no gaps has none.
func (model *Model) gapState(index int) (span gapSpan, shown, hidden int) {
	span = model.gapSpan(index)
	if !span.known {
		return span, 0, -1
	}
	shown = min(model.revealed[index], span.size())
	return span, shown, span.size() - shown
}

// gapHunk is the hunk the rows of gap index belong to: the one whose lines they
// continue.
func gapHunk(index int) int { return max(index-1, 0) }

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

// appendRow adds a row to base, numbering it.
func (model *Model) appendRow(added row) {
	added.baseIndex = len(model.base)
	model.base = append(model.base, added)
}

// resetRowCaches forgets everything kept by base row index, which a change of
// the base rows makes wrong.
func (model *Model) resetRowCaches() {
	model.cache = map[int][]Span{}
	model.highlighted = map[int]bool{}
	model.wholeRows = map[int]wholeRow{}
	model.wrapStarts = nil
	model.maximumWidth, model.numberWidth = 0, 0
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

// HasFileText reports whether SetFileText has given the text of the shown
// file, which ExpandGap needs.
func (model *Model) HasFileText() bool { return model.fileLines != nil }

// SetFileText gives the text of the shown file at the diff's head, so gaps can
// be expanded and the lines below the last hunk counted. It fails, and keeps
// the gaps closed, for a binary file or one over MaximumFileSize. Text given
// for a file that has no gaps is ignored. Another file, or a refresh of this
// one, forgets it.
func (model *Model) SetFileText(text string) error {
	if !canExpand(model.file) {
		return nil
	}
	switch {
	case len(text) > MaximumFileSize:
		return ErrFileTooLarge
	case strings.IndexByte(text, 0) >= 0:
		return ErrFileBinary
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // the newline that ends the last line
	}
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	model.fileLines = lines
	model.rebuild()
	return nil
}

// ExpandGap shows up to gapChunk more of the lines hidden at the gap under the
// cursor, all of them when that many or fewer are left. The cursor stays on
// the gap row while it remains, else it moves to the first line shown. It does
// nothing unless the cursor is on a gap and the file's text is known
// (HasFileText).
func (model *Model) ExpandGap() {
	gap := model.cursorGap()
	if gap == nil || model.fileLines == nil {
		return
	}
	if model.revealed == nil {
		model.revealed = map[int]int{}
	}
	model.revealed[gap.index] = min(model.revealed[gap.index]+gapChunk, model.gapSpan(gap.index).size())
	model.rebuild()
}

// rebuild lays the base rows out again after the gaps changed, keeping the
// cursor and its place in the view: on the gap row it was on while that
// remains, else on the diff line or thread it was on.
func (model *Model) rebuild() {
	cursor := model.window.Cursor()
	cursorOffset := cursor - model.window.Top()
	onGap, saved, savedOK := -1, position{}, false
	if cursor < len(model.rows) {
		if gap := model.rows[cursor].gap; gap != nil {
			onGap = gap.index
		} else {
			saved, savedOK = model.capture(), true
		}
	}
	model.visual = false // the base rows it points into are about to change
	model.resetRowCaches()
	model.base = nil
	model.build(model.file)
	switch {
	case onGap >= 0:
		// While the gap remains the cursor stays on its row. When it goes, the
		// cursor lands on the first line shown: where its row was, except for
		// a gap above the first hunk, whose lines come after the hunk's header.
		target := min(cursor, len(model.rows)-1)
		if row := model.gapRowIndex(onGap); row >= 0 {
			target = row
		} else if onGap == 0 && len(model.hunkRow) > 0 {
			target = min(model.hunkRow[0]+1, len(model.rows)-1)
		}
		model.window.Restore(target, target-cursorOffset, model.window.HorizontalOffset())
	case savedOK:
		model.restore(saved)
	}
}

// skipLeadingGap puts the cursor on the first row that is not a gap, so a file
// opens on its first hunk's header as it did before gaps were drawn; the gap
// above it is one row up.
func (model *Model) skipLeadingGap() {
	for i, row := range model.rows {
		if row.gap == nil {
			model.window.GotoRow(i)
			return
		}
	}
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
	text := ansi.Truncate(mark+model.relativeCell(i)+spaces(model.lineNumbersWidth())+gapText+hiddenText(row.gap.hidden), model.width, "")
	text += spaces(model.width - ansi.StringWidth(text))
	return model.styles.placeholder[current].Render(text)
}
