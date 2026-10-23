package diffview

import (
	"errors"
	"slices"

	"github.com/MikePors/strigidae/internal/diff"
)

// markSelected is the gutter mark of a selected row that is not the cursor row.
const markSelected = "▎"

// Range is the lines of one file a comment is written on. Start and End are
// line numbers on Side (the old file for SideLeft, the new file for
// SideRight), and are equal for a single line.
type Range struct {
	Path       string
	Side       Side
	Start, End int
}

// Why a selection cannot grow: one comment covers one hunk and one side.
var (
	// ErrCrossesHunk is returned when a range would cross into another hunk.
	ErrCrossesHunk = errors.New("a range must stay within one hunk")
	// ErrMixesSides is returned when a range would mix added and deleted lines.
	ErrMixesSides = errors.New("a range must stay on one side: it cannot mix added and deleted lines")

	errNoLines = errors.New("a range needs at least one diff line")
)

// VisualActive reports whether a line selection is in progress.
func (model *Model) VisualActive() bool { return model.vis }

// VisualStart begins a line selection at the cursor line. It reports false
// (and starts nothing) when the cursor is not on a diff line.
func (model *Model) VisualStart() bool {
	if _, ok := model.Cursor(); !ok {
		return false
	}
	model.vis, model.vbi = true, model.rows[model.win.Cursor()].bi
	return true
}

// VisualEnd leaves the line selection.
func (model *Model) VisualEnd() { model.vis = false }

// VisualMove moves the selection's free end by delta diff lines (negative is
// up), skipping thread rows. It stops before a line that would leave the hunk
// or mix added and deleted lines. When it cannot move at all, it returns why;
// a move that got part of the way is not an error.
func (model *Model) VisualMove(delta int) error {
	if !model.vis {
		return nil
	}
	from := model.win.Cursor()
	row, err := model.furthest(from, delta)
	if row == from {
		return err
	}
	model.win.GotoRow(row)
	return nil
}

// furthest walks up to |delta| diff lines from row in delta's direction and
// returns the last row whose selection from the anchor is valid, with the
// error that stopped the walk (nil when it ran out of lines or got all the
// way).
func (model *Model) furthest(from, delta int) (row int, err error) {
	step := 1
	if delta < 0 {
		step = -1
	}
	row = from
	for range abs(delta) {
		next := model.nextLineRow(row, step)
		if next < 0 {
			return row, nil
		}
		if _, err := model.rangeOf(model.vbi, model.rows[next].bi); err != nil {
			return row, err
		}
		row = next
	}
	return row, nil
}

// nextLineRow returns the closest row after (step 1) or before (step -1) row
// i that is a diff line, or -1.
func (model *Model) nextLineRow(i, step int) int {
	for j := i + step; j >= 0 && j < len(model.rows); j += step {
		if model.rows[j].isLine() {
			return j
		}
	}
	return -1
}

// Anchor returns the range a comment would be written on: the selection while
// one is in progress, else the cursor line. ok is false when the cursor is not
// on a diff line.
func (model *Model) Anchor() (lineRange Range, ok bool) {
	if model.vis {
		return model.selection()
	}
	cursorLine, ok := model.Cursor()
	if !ok {
		return Range{}, false
	}
	count := cursorLine.Number()
	return Range{Path: cursorLine.Path, Side: cursorLine.Side, Start: count, End: count}, true
}

// selection is the range between the selection's anchor and the cursor.
func (model *Model) selection() (Range, bool) {
	baseIndex, ok := model.cursorBase()
	if !ok {
		return Range{}, false
	}
	lineRange, err := model.rangeOf(model.vbi, baseIndex)
	return lineRange, err == nil
}

// cursorBase returns the base row the cursor is on, or the one it follows when
// it is on a thread row. ok is false when there are no rows.
func (model *Model) cursorBase() (baseIndex int, ok bool) {
	count := model.win.Cursor()
	if count >= len(model.rows) {
		return 0, false
	}
	return model.rows[count].bi, true
}

// rangeOf is the range between two base rows, or why there is none.
func (model *Model) rangeOf(first, count int) (Range, error) {
	low, high := min(first, count), max(first, count)
	lines, err := model.linesIn(low, high)
	if err != nil {
		return Range{}, err
	}
	return rangeFor(model.file.Path(), lines)
}

// linesIn collects the diff lines of base rows lo to hi, which must all be in
// one hunk.
func (model *Model) linesIn(low, high int) ([]*diff.Line, error) {
	hunk := model.base[low].hunk
	var lines []*diff.Line
	for _, row := range model.base[low : high+1] {
		if row.hunk != hunk {
			return nil, ErrCrossesHunk
		}
		if row.isLine() {
			lines = append(lines, row.line)
		}
	}
	return lines, nil
}

// rangeFor is the range covering lines (all of one hunk). A range containing a
// deleted line is on the old side, any other on the new side (context lines
// exist on both); added and deleted lines together are on no side.
func rangeFor(path string, lines []*diff.Line) (Range, error) {
	if len(lines) == 0 {
		return Range{}, errNoLines
	}
	isKind := func(k diff.LineKind) func(*diff.Line) bool {
		return func(line *diff.Line) bool { return line.Kind == k }
	}
	hasDel, hasAdd := slices.ContainsFunc(lines, isKind(diff.Delete)), slices.ContainsFunc(lines, isKind(diff.Add))
	if hasDel && hasAdd {
		return Range{}, ErrMixesSides
	}
	first, last := lines[0], lines[len(lines)-1]
	if hasDel {
		return Range{Path: path, Side: SideLeft, Start: first.Old, End: last.Old}, nil
	}
	return Range{Path: path, Side: SideRight, Start: first.New, End: last.New}, nil
}

// selected reports whether base row bi is inside the selection.
func (model *Model) selected(baseIndex int) bool {
	count, ok := model.cursorBase()
	return model.vis && ok && baseIndex >= min(model.vbi, count) && baseIndex <= max(model.vbi, count)
}
