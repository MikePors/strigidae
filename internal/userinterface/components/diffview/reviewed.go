package diffview

import (
	"slices"

	"github.com/MikePors/strigidae/internal/diff"
)

// markerReviewed starts the header row of a hunk the user marked reviewed.
const markerReviewed = "✓"

// SetHunkReviewed sets how the pane learns which hunks are marked reviewed:
// reviewed reports it for a hunk of a file, by index into file.Hunks. It is
// asked while rendering, so a change shows on the next render. nil marks none.
func (model *Model) SetHunkReviewed(reviewed func(file *diff.File, hunk int) bool) {
	model.hunkReviewed = reviewed
}

// CursorHunk returns the shown file and the index of the hunk the cursor is
// in: on its header, on one of its lines, or on a thread below one of them.
// ok is false with no file, on a notice, or on a thread that is on no line.
func (model *Model) CursorHunk() (file *diff.File, hunk int, ok bool) {
	if model.file == nil || len(model.rows) == 0 { // SetMessage clears the file
		return nil, 0, false
	}
	cursorRow := model.rows[model.window.Cursor()]
	if cursorRow.threadRow != nil {
		orphan := slices.Contains(model.orphans, cursorRow.threadRow.threadIndex)
		if orphan || cursorRow.baseIndex < 0 || cursorRow.baseIndex >= len(model.base) {
			return nil, 0, false
		}
		cursorRow = model.base[cursorRow.baseIndex]
	}
	if cursorRow.message {
		return nil, 0, false
	}
	return model.file, cursorRow.hunk, true
}
