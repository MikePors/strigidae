// Package scroll holds the bookkeeping for a cursor inside a scrolling window:
// which row the cursor is on, which row is at the top of the view, and how far
// the content is scrolled sideways. It knows nothing about what the rows are,
// how they are drawn, or which keys drive it, so list-like components can share
// it and a host maps keys to its methods.
//
// Usage:
//
//	var w scroll.Window
//	w.SetRows(len(items))
//	w.SetSize(height)
//	w.SetWidths(widestItem, viewWidth)
//	w.Down(1)
//	for i := w.Top(); i < min(w.Top()+height, len(items)); i++ { /* draw; cursor row is w.Cursor() */ }
//
// Every method leaves the window clamped: the cursor is a valid row, the cursor
// is inside the view, the top row is no further than the last full page, and
// the horizontal offset never scrolls past the end of the widest content.
package scroll

// Window is a cursor, a top row and a horizontal offset over a list of rows.
// The zero value is an empty window of height 0.
type Window struct {
	rows, height  int
	content, view int // widest content and visible width, in cells

	cursor, top, x int
}

// SetRows sets the number of rows and re-clamps.
func (window *Window) SetRows(count int) { window.rows = max(count, 0); window.clamp() }

// SetSize sets the number of visible rows and re-clamps.
func (window *Window) SetSize(height int) { window.height = max(height, 0); window.clamp() }

// SetWidths sets the width of the widest content row and the visible width,
// both in cells, and re-clamps the horizontal offset.
func (window *Window) SetWidths(content, view int) {
	window.content, window.view = max(content, 0), max(view, 0)
	window.clamp()
}

// Reset puts the cursor, top and horizontal offset back at zero.
func (window *Window) Reset() { window.cursor, window.top, window.x = 0, 0, 0 }

// Restore sets a saved position and clamps it to the current bounds.
func (window *Window) Restore(cursor, top, column int) {
	window.cursor, window.top, window.x = cursor, top, column
	window.clamp()
}

// Cursor returns the cursor row.
func (window *Window) Cursor() int { return window.cursor }

// Top returns the first visible row.
func (window *Window) Top() int { return window.top }

// XOffset returns the horizontal scroll in cells.
func (window *Window) HorizontalOffset() int { return window.x }

// Height returns the number of visible rows.
func (window *Window) Height() int { return window.height }

// MaxTop returns the largest valid top row.
func (window *Window) MaximumTop() int { return max(window.rows-window.height, 0) }

// MaxX returns the largest valid horizontal offset.
func (window *Window) MaximumHorizontalOffset() int { return max(window.content-window.view, 0) }

func (window *Window) clamp() {
	if window.rows == 0 {
		window.top, window.cursor, window.x = 0, 0, 0
		return
	}
	window.cursor = min(max(window.cursor, 0), window.rows-1)
	window.top = min(max(window.top, 0), window.MaximumTop())
	window.x = min(max(window.x, 0), window.MaximumHorizontalOffset())
	window.follow()
}

// follow scrolls the minimum needed to keep the cursor visible.
func (window *Window) follow() {
	if window.height <= 0 {
		return
	}
	if window.cursor < window.top {
		window.top = window.cursor
	}
	if window.cursor >= window.top+window.height {
		window.top = window.cursor - window.height + 1
	}
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

// GotoRow moves the cursor to row r, scrolling to keep it visible.
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
	window.top = min(max(window.top+count, 0), window.MaximumTop())
	window.cursor = min(max(window.cursor, window.top), max(window.top+window.height-1, window.top))
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

func (window *Window) shift(count int) {
	window.top = min(max(window.top+count, 0), window.MaximumTop())
	window.cursor += count
	window.clamp()
}

// --- horizontal scrolling ---

// Right shifts the content n cells to the right (reveals later text).
func (window *Window) Right(count int) { window.x += count; window.clamp() }

// Left shifts the content n cells to the left.
func (window *Window) Left(count int) { window.x -= count; window.clamp() }

// HalfRight scrolls right by half the visible width.
func (window *Window) HalfRight() { window.Right(max(window.view/2, 1)) }

// HalfLeft scrolls left by half the visible width.
func (window *Window) HalfLeft() { window.Left(max(window.view/2, 1)) }
