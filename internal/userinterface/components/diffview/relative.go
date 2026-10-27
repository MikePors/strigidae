package diffview

import (
	"strconv"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

// relativeWidth is the width of the relative-number column: the digits of
// keys.MaximumCount. Counts stop there, so a row further away is left blank.
const relativeWidth = 4

// WithRelativeNumber starts the pane with the relative-number column shown.
func WithRelativeNumber(on bool) Option { return func(model *Model) { model.relativeNumber = on } }

// SetRelativeNumber shows or hides the column that numbers every row by its
// distance from the cursor, so a count with j or k lands on the row it names.
func (model *Model) SetRelativeNumber(on bool) {
	if on == model.relativeNumber {
		return
	}
	model.relativeNumber = on
	model.layout() // thread blocks are drawn to the narrower width
}

// relativeColumnWidth is the width the column takes in the gutter: the
// numbers and a space, or nothing when it is hidden.
func (model *Model) relativeColumnWidth() int {
	if !model.relativeNumber {
		return 0
	}
	return relativeWidth + 1
}

// relativeCell returns row i's cell of the column: its distance from the
// cursor, right-aligned, then a space.
func (model *Model) relativeCell(i int) string {
	if !model.relativeNumber {
		return ""
	}
	distance, text := absolute(i-model.window.Cursor()), ""
	if distance <= keys.MaximumCount {
		text = strconv.Itoa(distance)
	}
	return spaces(relativeWidth-len(text)) + text + " "
}
