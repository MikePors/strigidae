package diffview

import "github.com/MikePors/strigidae/internal/diff"

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

// Refusals shown when a selection would leave what one comment can cover.
const (
	// RefuseHunk is the reason when a range would cross into another hunk.
	RefuseHunk = "a range must stay within one hunk"
	// RefuseSide is the reason when a range would mix added and deleted lines.
	RefuseSide = "a range must stay on one side: it cannot mix added and deleted lines"
)

// VisualActive reports whether a line selection is in progress.
func (m *Model) VisualActive() bool { return m.vis }

// VisualStart begins a line selection at the cursor line. It reports false
// (and starts nothing) when the cursor is not on a diff line.
func (m *Model) VisualStart() bool {
	if _, ok := m.Cursor(); !ok {
		return false
	}
	m.vis, m.vbi = true, m.rows[m.win.Cursor()].bi
	return true
}

// VisualEnd leaves the line selection.
func (m *Model) VisualEnd() { m.vis = false }

// VisualMove moves the selection's free end by delta diff lines (negative is
// up), skipping thread rows. A move that would leave the hunk, or mix added
// and deleted lines, is refused: nothing moves and the reason is returned.
func (m *Model) VisualMove(delta int) (refusal string) {
	if !m.vis || delta == 0 {
		return ""
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	cur := m.win.Cursor()
	for n := 0; n != delta; n += step {
		next := m.nextLineRow(cur, step)
		if next < 0 {
			break
		}
		if _, why := m.rangeOf(m.vbi, m.rows[next].bi); why != "" {
			if n == 0 {
				return why
			}
			break
		}
		cur = next
	}
	m.win.Down(cur - m.win.Cursor()) // negative n moves up through Down's clamp
	return ""
}

// nextLineRow returns the closest row after (step 1) or before (step -1) row
// i that is a diff line, or -1.
func (m *Model) nextLineRow(i, step int) int {
	for j := i + step; j >= 0 && j < len(m.rows); j += step {
		if r := m.rows[j]; r.tr == nil && r.line != nil && r.line.Kind != diff.NoNewline {
			return j
		}
	}
	return -1
}

// Anchor returns the range a comment would be written on: the selection while
// one is in progress, else the cursor line. ok is false when the cursor is not
// on a diff line.
func (m *Model) Anchor() (r Range, ok bool) {
	if m.vis && m.win.Cursor() < len(m.rows) {
		r, why := m.rangeOf(m.vbi, m.rows[m.win.Cursor()].bi)
		return r, why == ""
	}
	c, ok := m.Cursor()
	if !ok {
		return Range{}, false
	}
	n := c.Number()
	return Range{Path: c.Path, Hunk: c.Hunk, Side: c.Side, Start: n, End: n}, true
}

// rangeOf is the range between two base rows, or the reason there is none.
// A range containing a deleted line is on the old side, any other on the new
// side; context lines exist on both.
func (m *Model) rangeOf(a, b int) (Range, string) {
	lo, hi := min(a, b), max(a, b)
	var first, last *diff.Line
	hunk, add, del := m.base[lo].hunk, false, false
	for i := lo; i <= hi; i++ {
		r := m.base[i]
		if r.hunk != hunk {
			return Range{}, RefuseHunk
		}
		if r.line == nil || r.line.Kind == diff.NoNewline {
			continue
		}
		add = add || r.line.Kind == diff.Add
		del = del || r.line.Kind == diff.Delete
		if first == nil {
			first = r.line
		}
		last = r.line
	}
	if first == nil {
		return Range{}, RefuseHunk
	}
	if add && del {
		return Range{}, RefuseSide
	}
	out := Range{Path: m.file.Path(), Hunk: hunk, Side: SideRight, Start: first.New, End: last.New}
	if del {
		out.Side, out.Start, out.End = SideLeft, first.Old, last.Old
	}
	return out, ""
}

// selected reports whether base row bi is inside the selection.
func (m *Model) selected(bi int) bool {
	if !m.vis || m.win.Cursor() >= len(m.rows) {
		return false
	}
	c := m.rows[m.win.Cursor()].bi
	return bi >= min(m.vbi, c) && bi <= max(m.vbi, c)
}
