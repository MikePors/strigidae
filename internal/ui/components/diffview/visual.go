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
	Hunk       int
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
func (m *Model) VisualActive() bool { return m.vis }

// VisualStart begins a line selection at the cursor line. It reports false
// (and starts nothing) when the cursor is not on a diff line.
func (m *Model) VisualStart() bool {
	bi, ok := m.cursorBase()
	if _, onLine := m.Cursor(); !ok || !onLine {
		return false
	}
	m.vis, m.vbi = true, bi
	return true
}

// VisualEnd leaves the line selection.
func (m *Model) VisualEnd() { m.vis = false }

// VisualMove moves the selection's free end by delta diff lines (negative is
// up), skipping thread rows. It stops before a line that would leave the hunk
// or mix added and deleted lines. When it cannot move at all, it returns why;
// a move that got part of the way is not an error.
func (m *Model) VisualMove(delta int) error {
	if !m.vis {
		return nil
	}
	from := m.win.Cursor()
	row, err := m.furthest(from, delta)
	if row == from {
		return err
	}
	m.win.GotoRow(row)
	return nil
}

// furthest walks up to |delta| diff lines from row in delta's direction and
// returns the last row whose selection from the anchor is valid, with the
// error that stopped the walk (nil when it ran out of lines or got all the
// way).
func (m *Model) furthest(from, delta int) (row int, err error) {
	step := 1
	if delta < 0 {
		step = -1
	}
	row = from
	for range abs(delta) {
		next := m.nextLineRow(row, step)
		if next < 0 {
			return row, nil
		}
		if _, err := m.rangeOf(m.vbi, m.rows[next].bi); err != nil {
			return row, err
		}
		row = next
	}
	return row, nil
}

// nextLineRow returns the closest row after (step 1) or before (step -1) row
// i that is a diff line, or -1.
func (m *Model) nextLineRow(i, step int) int {
	for j := i + step; j >= 0 && j < len(m.rows); j += step {
		if m.rows[j].isLine() {
			return j
		}
	}
	return -1
}

// Anchor returns the range a comment would be written on: the selection while
// one is in progress, else the cursor line. ok is false when the cursor is not
// on a diff line.
func (m *Model) Anchor() (r Range, ok bool) {
	if m.vis {
		return m.selection()
	}
	c, ok := m.Cursor()
	if !ok {
		return Range{}, false
	}
	n := c.Number()
	return Range{Path: c.Path, Hunk: c.Hunk, Side: c.Side, Start: n, End: n}, true
}

// selection is the range between the selection's anchor and the cursor.
func (m *Model) selection() (Range, bool) {
	bi, ok := m.cursorBase()
	if !ok {
		return Range{}, false
	}
	r, err := m.rangeOf(m.vbi, bi)
	return r, err == nil
}

// cursorBase returns the base row the cursor is on, or the one it follows when
// it is on a thread row. ok is false when there are no rows.
func (m *Model) cursorBase() (bi int, ok bool) {
	c := m.win.Cursor()
	if c >= len(m.rows) {
		return 0, false
	}
	return m.rows[c].bi, true
}

// rangeOf is the range between two base rows, or why there is none.
func (m *Model) rangeOf(a, b int) (Range, error) {
	lo, hi := min(a, b), max(a, b)
	lines, err := m.linesIn(lo, hi)
	if err != nil {
		return Range{}, err
	}
	return rangeFor(m.file.Path(), m.base[lo].hunk, lines)
}

// linesIn collects the diff lines of base rows lo to hi, which must all be in
// one hunk.
func (m *Model) linesIn(lo, hi int) ([]*diff.Line, error) {
	hunk := m.base[lo].hunk
	var lines []*diff.Line
	for _, r := range m.base[lo : hi+1] {
		if r.hunk != hunk {
			return nil, ErrCrossesHunk
		}
		if r.isLine() {
			lines = append(lines, r.line)
		}
	}
	return lines, nil
}

// rangeFor is the range covering lines, all of one hunk. A range containing a
// deleted line is on the old side, any other on the new side (context lines
// exist on both); added and deleted lines together are on no side.
func rangeFor(path string, hunk int, lines []*diff.Line) (Range, error) {
	if len(lines) == 0 {
		return Range{}, errNoLines
	}
	is := func(k diff.LineKind) func(*diff.Line) bool {
		return func(l *diff.Line) bool { return l.Kind == k }
	}
	hasDel, hasAdd := slices.ContainsFunc(lines, is(diff.Delete)), slices.ContainsFunc(lines, is(diff.Add))
	if hasDel && hasAdd {
		return Range{}, ErrMixesSides
	}
	first, last := lines[0], lines[len(lines)-1]
	if hasDel {
		return Range{Path: path, Hunk: hunk, Side: SideLeft, Start: first.Old, End: last.Old}, nil
	}
	return Range{Path: path, Hunk: hunk, Side: SideRight, Start: first.New, End: last.New}, nil
}

// selected reports whether base row bi is inside the selection.
func (m *Model) selected(bi int) bool {
	c, ok := m.cursorBase()
	return m.vis && ok && bi >= min(m.vbi, c) && bi <= max(m.vbi, c)
}
