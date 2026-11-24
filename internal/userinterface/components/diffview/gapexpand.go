package diffview

import "github.com/MikePors/strigidae/internal/diff"

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
	span := model.gapSpan(gap.index)
	before := min(model.revealed[gap.index], span.size())
	after := min(before+gapChunk, span.size())
	model.revealed[gap.index] = after
	// The first line this shows: the top gap opens upward from the hunk,
	// the others downward from the one above.
	focus := span.first + before
	if gap.index == 0 {
		focus = span.last - after + 1
	}
	model.rebuild(focus)
}

// rebuild lays the base rows out again after the gaps changed, keeping the
// cursor and its place in the view: on the gap row it was on while that
// remains, else on the shown line numbered focus (the first one the gap's
// expansion shows), else on the diff line or thread it was on.
func (model *Model) rebuild(focus int) {
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
	// A selection survives, found again by its diff line, unless the cursor
	// was on a gap, which a selection never rests on.
	var selected *diff.Line
	if model.visual && onGap < 0 && model.visualBaseIndex < len(model.base) {
		selected = model.base[model.visualBaseIndex].line
	}
	model.visual = false
	model.resetRowCaches()
	model.base = nil
	model.build(model.file)
	if selected != nil {
		for i, row := range model.base {
			if row.line == selected {
				model.visual, model.visualBaseIndex = true, i
				break
			}
		}
	}
	switch {
	case onGap >= 0:
		// While the gap remains the cursor stays on its row; when it goes,
		// it lands on the first line shown.
		target := min(cursor, len(model.rows)-1)
		if row := model.gapRowIndex(onGap); row >= 0 {
			target = row
		} else if row := model.shownRow(focus); row >= 0 {
			target = row
		}
		model.window.Restore(target, target-cursorOffset, model.window.HorizontalOffset())
	case savedOK:
		model.restore(saved)
	}
}

// shownRow returns the row of the line shown by a gap that has the new-side
// number, or -1.
func (model *Model) shownRow(number int) int {
	for i, row := range model.rows {
		if row.revealed && row.line.New == number {
			return i
		}
	}
	return -1
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

// gapAnchor names a gap by the hunk below it, so it can be found again after
// the diff is re-parsed and hunks come or go above it.
type gapAnchor struct {
	last             bool // the gap below the last hunk
	hunkOld, hunkNew int
}

func (model *Model) gapAnchorOf(index int) gapAnchor {
	if index >= len(model.file.Hunks) {
		return gapAnchor{last: true}
	}
	hunk := model.file.Hunks[index]
	return gapAnchor{hunkOld: hunk.OldStart, hunkNew: hunk.NewStart}
}

// gapRowAt returns the row of the gap anchored to the same hunk, or -1.
func (model *Model) gapRowAt(anchor gapAnchor) int {
	for i, row := range model.rows {
		if row.gap != nil && model.gapAnchorOf(row.gap.index) == anchor {
			return i
		}
	}
	return -1
}
