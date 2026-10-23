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
	path             string
	anchor           anchor
	cursorOffset     int // cursor row minus top row
	horizontalOffset int
	ok               bool // false when there was no diff row to anchor to

	// thread and toff say where in a thread's rows the cursor was, so it can
	// go back there when the thread is still shown.
	thread       string
	threadOffset int
}

func (model *Model) capture() position {
	currentPosition := position{path: model.file.Path(), horizontalOffset: model.window.HorizontalOffset()}
	if len(model.rows) == 0 {
		return currentPosition
	}
	i := model.window.Cursor()
	currentPosition.cursorOffset = model.window.Cursor() - model.window.Top()
	if row := model.rows[i]; row.threadRow != nil {
		currentPosition.thread, currentPosition.threadOffset = model.fileThreads[row.threadRow.threadIndex].Identifier, row.threadRow.rowInBlock
	}
	for i > 0 && model.rows[i].threadRow != nil {
		i-- // a thread row: anchor to the diff row it follows
	}
	if row := model.rows[i]; row.line != nil && row.line.Kind == diff.NoNewline && i > 0 {
		i-- // markers have no numbers; anchor to the line they belong to
	}
	if row := model.rows[i]; row.threadRow != nil || row.message {
		return currentPosition // no diff row to anchor to
	}
	currentPosition.anchor = model.anchorAt(i)
	currentPosition.ok = true
	return currentPosition
}

func (model *Model) anchorAt(i int) anchor {
	row := model.rows[i]
	if row.line == nil {
		hunk := model.file.Hunks[row.hunk]
		return anchor{header: true, hunkOld: hunk.OldStart, hunkNew: hunk.NewStart}
	}
	return anchor{kind: row.line.Kind, old: row.line.Old, new: row.line.New}
}

// restore puts the cursor back on the row matching position, or on the nearest one.
func (model *Model) restore(position position) {
	if len(model.rows) == 0 {
		return
	}
	row := -1
	if position.thread != "" {
		row = model.threadRowFor(position.thread, position.threadOffset)
	}
	if row < 0 {
		if !position.ok {
			return
		}
		row = model.find(position.anchor)
	}
	model.window.Restore(row, row-position.cursorOffset, position.horizontalOffset)
}

func lineKey(oldNumber, newNumber int) int {
	if newNumber > 0 {
		return newNumber
	}
	return oldNumber
}

func absolute(count int) int {
	if count < 0 {
		return -count
	}
	return count
}

// find returns the row for anchor: an exact match on kind and line numbers, else
// the row of the same sort whose line number is closest.
func (model *Model) find(anchor anchor) int {
	best, bestDistance := 0, -1
	for i, row := range model.rows {
		var distance int
		switch {
		case anchor.header:
			if row.line != nil || row.threadRow != nil || row.message {
				continue
			}
			hunk := model.file.Hunks[row.hunk]
			if hunk.OldStart == anchor.hunkOld && hunk.NewStart == anchor.hunkNew {
				return i
			}
			distance = absolute(lineKey(hunk.OldStart, hunk.NewStart) - lineKey(anchor.hunkOld, anchor.hunkNew))
		default:
			if !row.isLine() { // also skips thread rows
				continue
			}
			if row.line.Kind == anchor.kind && row.line.Old == anchor.old && row.line.New == anchor.new {
				return i
			}
			distance = absolute(lineKey(row.line.Old, row.line.New) - lineKey(anchor.old, anchor.new))
		}
		if bestDistance < 0 || distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	return best
}
