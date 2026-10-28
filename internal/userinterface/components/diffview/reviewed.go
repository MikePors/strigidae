package diffview

import "github.com/MikePors/strigidae/internal/diff"

// markerReviewed starts the header row of a hunk the user marked reviewed.
const markerReviewed = "✓"

// SetHunkReviewed sets how the pane learns which hunks are marked reviewed:
// reviewed reports it for a hunk of a file, by index into file.Hunks. It is
// asked while rendering, so a change shows on the next render. nil marks none.
func (model *Model) SetHunkReviewed(reviewed func(file *diff.File, hunk int) bool) {
	model.hunkReviewed = reviewed
}
