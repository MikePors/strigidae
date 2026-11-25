package diffview

import (
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// relativeDigits is how many digits the relative-number column holds: those
// of keys.MaximumCount. Counts stop there, so a row further away is left
// blank.
const relativeDigits = 4

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
	return relativeDigits + 1
}

// relativeStyles draws the relative-number cell on one row background.
type relativeStyles struct{ number, cursor lipgloss.Style }

func newRelativeStyles(activeTheme theme.Theme, background lipgloss.Style) relativeStyles {
	return relativeStyles{
		number: activeTheme.DiffRelativeNumber.Background(background.GetBackground()),
		cursor: activeTheme.DiffRelativeNumberCursor.Background(background.GetBackground()),
	}
}

// relativeCell returns row i's cell of the column, drawn with styles: its
// distance from the cursor, right-aligned, then a space.
func (model *Model) relativeCell(i int, styles relativeStyles) string {
	if !model.relativeNumber {
		return ""
	}
	distance, text, style := absolute(i-model.window.Cursor()), "", styles.number
	if distance == 0 {
		style = styles.cursor
	}
	if distance <= keys.MaximumCount {
		text = strconv.Itoa(distance)
	}
	return style.Render(spaces(relativeDigits-len(text)) + text + " ")
}

// markedRow draws row i in one style but for its relative-number cell: the
// mark, the cell, then text, cut or padded to the pane's width. A styled
// cell cannot sit inside the row's own Render, so it is drawn apart.
func (model *Model) markedRow(i int, mark, text string, style lipgloss.Style, relative relativeStyles) string {
	if !model.relativeNumber {
		text = ansi.Truncate(mark+text, model.width, "")
		return style.Render(text + spaces(model.width-ansi.StringWidth(text)))
	}
	rest := max(model.width-ansi.StringWidth(mark)-model.relativeColumnWidth(), 0)
	text = ansi.Truncate(text, rest, "")
	line := style.Render(mark) + model.relativeCell(i, relative) + style.Render(text+spaces(rest-ansi.StringWidth(text)))
	return ansi.Truncate(line, model.width, "")
}
