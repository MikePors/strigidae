package diffview

import "github.com/MikePors/strigidae/internal/diff"

// anchor identifies a cursor row by its diff line, not its row index, so the
// cursor can be put back on the same line after the diff is re-parsed.
type anchor struct {
	header           bool // a hunk header row
	kind             diff.LineKind
	old, new         int // line numbers (hunk start numbers for headers)
	hunkOld, hunkNew int
}

// position is a saved view position for one file path.
type position struct {
	path string
	a    anchor
	off  int // cursor row minus top row
	xoff int
	ok   bool // false when there was no diff row to anchor to

	// thread and toff say where in a thread's rows the cursor was, so it can
	// go back there when the thread is still shown.
	thread string
	toff   int
}

func (m *Model) capture() position {
	p := position{path: m.file.Path(), xoff: m.win.XOffset()}
	if len(m.rows) == 0 {
		return p
	}
	i := m.win.Cursor()
	p.off = m.win.Cursor() - m.win.Top()
	if r := m.rows[i]; r.tr != nil {
		p.thread, p.toff = m.fthreads[r.tr.t].ID, r.tr.i
	}
	for i > 0 && m.rows[i].tr != nil {
		i-- // a thread row: anchor to the diff row it follows
	}
	if r := m.rows[i]; r.line != nil && r.line.Kind == diff.NoNewline && i > 0 {
		i-- // markers have no numbers; anchor to the line they belong to
	}
	if r := m.rows[i]; r.tr != nil || r.msg {
		return p // no diff row to anchor to
	}
	p.a = m.anchorAt(i)
	p.ok = true
	return p
}

func (m *Model) anchorAt(i int) anchor {
	r := m.rows[i]
	if r.line == nil {
		h := m.file.Hunks[r.hunk]
		return anchor{header: true, hunkOld: h.OldStart, hunkNew: h.NewStart}
	}
	return anchor{kind: r.line.Kind, old: r.line.Old, new: r.line.New}
}

// restore puts the cursor back on the row matching p, or on the nearest one.
func (m *Model) restore(p position) {
	if len(m.rows) == 0 {
		return
	}
	row := -1
	if p.thread != "" {
		row = m.threadRowFor(p.thread, p.toff)
	}
	if row < 0 {
		if !p.ok {
			return
		}
		row = m.find(p.a)
	}
	m.win.Restore(row, row-p.off, p.xoff)
}

func lineKey(oldNo, newNo int) int {
	if newNo > 0 {
		return newNo
	}
	return oldNo
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// find returns the row for a: an exact match on kind and line numbers, else
// the row of the same sort whose line number is closest.
func (m *Model) find(a anchor) int {
	best, bestDist := 0, -1
	for i, r := range m.rows {
		var dist int
		switch {
		case a.header:
			if r.line != nil || r.tr != nil || r.msg {
				continue
			}
			h := m.file.Hunks[r.hunk]
			if h.OldStart == a.hunkOld && h.NewStart == a.hunkNew {
				return i
			}
			dist = abs(lineKey(h.OldStart, h.NewStart) - lineKey(a.hunkOld, a.hunkNew))
		default:
			if !r.isLine() { // also skips thread rows
				continue
			}
			if r.line.Kind == a.kind && r.line.Old == a.old && r.line.New == a.new {
				return i
			}
			dist = abs(lineKey(r.line.Old, r.line.New) - lineKey(a.old, a.new))
		}
		if bestDist < 0 || dist < bestDist {
			best, bestDist = i, dist
		}
	}
	return best
}
