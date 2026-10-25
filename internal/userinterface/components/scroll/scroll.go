// Package scroll holds the bookkeeping for a cursor inside a scrolling window:
// which row the cursor is on, which row is at the top of the view, and how far
// the content is scrolled sideways. It knows nothing about what the rows are,
// how they are drawn, or which keys drive it, so list-like components can share
// it and a host maps keys to its methods.
//
// Usage:
//
//	var window scroll.Window
//	window.SetRows(len(items))
//	window.SetSize(height)
//	window.SetWidths(widestItem, viewWidth)
//	window.Down(1)
//	for i := window.Top(); i < min(window.Top()+height, len(items)); i++ { /* draw; cursor row is window.Cursor() */ }
//
// Every method leaves the window clamped: the cursor is a valid row, the cursor
// is inside the view, the top row is no further than the last full page, and
// the horizontal offset never scrolls past the end of the widest content.
//
// A row is one screen row unless SetRowHeight says otherwise (wrapped text).
// The cursor still moves by rows; the view, its height and the screen
// positioning (ScrollCursorTop and friends) count screen rows, and the cursor
// row is kept fully visible when it fits.
package scroll

// Window is a cursor, a top row and a horizontal offset over a list of rows.
// The zero value is an empty window of height 0.
type Window struct {
	rows, height  int
	content, view int               // widest content and visible width, in cells
	rowHeight     func(row int) int // screen rows of each row; nil: one each

	cursor, top, column int
}

// SetRows sets the number of rows and re-clamps.
func (window *Window) SetRows(count int) { window.rows = max(count, 0); window.clamp() }

// SetSize sets the number of visible rows and re-clamps.
func (window *Window) SetSize(height int) { window.height = max(height, 0); window.clamp() }

// SetRowHeight sets how many screen rows each row takes, and re-clamps.
// height(row) is asked only for rows near the view, so it may compute lazily;
// a result below 1 counts as 1. nil (the default) makes every row one screen
// row.
func (window *Window) SetRowHeight(height func(row int) int) {
	window.rowHeight = height
	window.clamp()
}

// heightOf returns the screen rows row takes.
func (window *Window) heightOf(row int) int {
	if window.rowHeight == nil {
		return 1
	}
	return max(window.rowHeight(row), 1)
}

// SetWidths sets the width of the widest content row and the visible width,
// both in cells, and re-clamps the horizontal offset.
func (window *Window) SetWidths(content, view int) {
	window.content, window.view = max(content, 0), max(view, 0)
	window.clamp()
}

// Reset puts the cursor, top and horizontal offset back at zero.
func (window *Window) Reset() { window.cursor, window.top, window.column = 0, 0, 0 }

// Restore sets a saved position and clamps it to the current bounds.
func (window *Window) Restore(cursor, top, column int) {
	window.cursor, window.top, window.column = cursor, top, column
	window.clamp()
}

// Cursor returns the cursor row.
func (window *Window) Cursor() int { return window.cursor }

// Top returns the first visible row.
func (window *Window) Top() int { return window.top }

// HorizontalOffset returns the horizontal scroll in cells.
func (window *Window) HorizontalOffset() int { return window.column }

// Height returns the number of visible rows.
func (window *Window) Height() int { return window.height }

// MaximumTop returns the largest valid top row: the last rows fill the view
// from there.
func (window *Window) MaximumTop() int {
	if window.rowHeight == nil {
		return max(window.rows-window.height, 0)
	}
	used := 0
	for row := window.rows - 1; row >= 0; row-- {
		used += window.heightOf(row)
		if used > window.height {
			return min(row+1, window.rows-1)
		}
	}
	return 0
}

// LastVisible returns the last row that is fully on screen with top at the
// top of the view (top itself if even it does not fit).
func (window *Window) LastVisible() int { return window.lastVisible(window.top) }

func (window *Window) lastVisible(top int) int {
	if window.rowHeight == nil {
		return min(max(top+window.height-1, top), window.rows-1)
	}
	used := 0
	for row := top; row < window.rows; row++ {
		used += window.heightOf(row)
		if used > window.height {
			return max(row-1, top)
		}
	}
	return window.rows - 1
}

// MaximumHorizontalOffset returns the largest valid horizontal offset.
func (window *Window) MaximumHorizontalOffset() int { return max(window.content-window.view, 0) }

func (window *Window) clamp() {
	if window.rows == 0 {
		window.top, window.cursor, window.column = 0, 0, 0
		return
	}
	window.cursor = min(max(window.cursor, 0), window.rows-1)
	window.top = min(max(window.top, 0), window.MaximumTop())
	window.column = min(max(window.column, 0), window.MaximumHorizontalOffset())
	window.follow()
}

// follow scrolls the minimum needed to keep the cursor row visible.
func (window *Window) follow() {
	if window.height <= 0 {
		return
	}
	if window.cursor < window.top {
		window.top = window.cursor
	}
	// The first top that still shows the whole cursor row.
	first, used := window.cursor, window.heightOf(window.cursor)
	for first > 0 && used+window.heightOf(first-1) <= window.height {
		first--
		used += window.heightOf(first)
	}
	window.top = max(window.top, first)
}

// --- cursor movement: the view follows the cursor ---

// Down moves the cursor down n rows, scrolling to keep it visible.
func (window *Window) Down(count int) { window.cursor += count; window.clamp() }

// Up moves the cursor up n rows, scrolling to keep it visible.
func (window *Window) Up(count int) { window.cursor -= count; window.clamp() }

// GotoTop moves the cursor to the first row.
func (window *Window) GotoTop() { window.cursor = 0; window.clamp() }

// GotoBottom moves the cursor to the last row.
func (window *Window) GotoBottom() { window.cursor = window.rows - 1; window.clamp() }

// GotoRow moves the cursor to the given row, scrolling to keep it visible.
func (window *Window) GotoRow(row int) { window.cursor = row; window.clamp() }

// JumpTo puts the cursor on row r and r at the top of the view (as far as the
// last page allows).
func (window *Window) JumpTo(row int) { window.cursor, window.top = row, row; window.clamp() }

// --- viewport movement ---

// ScrollDown scrolls the view down n rows without moving the cursor, unless
// the cursor would leave the view, in which case it is pulled along.
func (window *Window) ScrollDown(count int) { window.scrollBy(count) }

// ScrollUp is the opposite of ScrollDown.
func (window *Window) ScrollUp(count int) { window.scrollBy(-count) }

func (window *Window) scrollBy(count int) {
	window.top = min(max(window.top+window.rowsFor(window.top, count), 0), window.MaximumTop())
	window.cursor = min(max(window.cursor, window.top), window.lastVisible(window.top))
	window.clamp()
}

// rowsFor returns how many rows the view moves past when it scrolls count
// screen rows from top (negative: up). It always moves at least one row when
// count is not zero, so a row taller than count cannot stall scrolling. Past
// the first or last row the remaining screen rows count as rows, which keeps
// the cursor moving at the ends as it does with one screen row per row.
func (window *Window) rowsFor(top, count int) int {
	step, remaining := 1, count
	if count < 0 {
		step, remaining = -1, -count
	}
	row := top
	for remaining > 0 {
		next := row + step
		if next < 0 || next >= window.rows {
			return row - top + step*remaining
		}
		leaving := row // scrolling down, the top row leaves the view
		if step < 0 {
			leaving = next // scrolling up, the row above enters it
		}
		height := window.heightOf(leaving)
		if height > remaining && row != top {
			break
		}
		remaining -= height
		row = next
	}
	return row - top
}

// ScrollCursorTop scrolls the view so the cursor row is its first row, as far
// as the end of the list allows. The cursor does not move.
func (window *Window) ScrollCursorTop() { window.placeCursor(0) }

// ScrollCursorMiddle scrolls the view so the cursor row is in the middle of
// it, as far as the ends of the list allow. The cursor does not move.
func (window *Window) ScrollCursorMiddle() {
	window.placeCursor((window.height - window.heightOf(window.cursor)) / 2)
}

// ScrollCursorBottom scrolls the view so the cursor row is its last row, as
// far as the start of the list allows. The cursor does not move. A row taller
// than one screen row is then pulled up whole by the clamp.
func (window *Window) ScrollCursorBottom() { window.placeCursor(window.height - 1) }

// placeCursor puts the cursor row's first screen row offset screen rows
// below the top of the view: as many rows above the cursor as fit there.
func (window *Window) placeCursor(offset int) {
	top, used := window.cursor, 0
	for top > 0 && used+window.heightOf(top-1) <= offset {
		top--
		used += window.heightOf(top)
	}
	window.top = min(top, window.MaximumTop())
	window.clamp()
}

// HalfPageDown moves the view and the cursor down half a screen, as in vim.
func (window *Window) HalfPageDown() { window.shift(max(window.height/2, 1)) }

// HalfPageUp moves the view and the cursor up half a screen.
func (window *Window) HalfPageUp() { window.shift(-max(window.height/2, 1)) }

// PageDown moves the view and the cursor down a full screen.
func (window *Window) PageDown() { window.shift(max(window.height, 1)) }

// PageUp moves the view and the cursor up a full screen.
func (window *Window) PageUp() { window.shift(-max(window.height, 1)) }

// shift moves the view count screen rows and the cursor by the same number
// of rows.
func (window *Window) shift(count int) {
	rows := window.rowsFor(window.top, count)
	window.top = min(max(window.top+rows, 0), window.MaximumTop())
	window.cursor += rows
	window.clamp()
}

// --- horizontal scrolling ---

// Right shifts the content n cells to the right (reveals later text).
func (window *Window) Right(count int) { window.column += count; window.clamp() }

// Left shifts the content n cells to the left.
func (window *Window) Left(count int) { window.column -= count; window.clamp() }

// HalfRight scrolls right by half the visible width.
func (window *Window) HalfRight() { window.Right(max(window.view/2, 1)) }

// HalfLeft scrolls left by half the visible width.
func (window *Window) HalfLeft() { window.Left(max(window.view/2, 1)) }
