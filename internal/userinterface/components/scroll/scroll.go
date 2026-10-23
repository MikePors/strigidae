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
func (w *Window) SetRows(n int) { w.rows = max(n, 0); w.clamp() }

// SetSize sets the number of visible rows and re-clamps.
func (w *Window) SetSize(height int) { w.height = max(height, 0); w.clamp() }

// SetWidths sets the width of the widest content row and the visible width,
// both in cells, and re-clamps the horizontal offset.
func (w *Window) SetWidths(content, view int) {
	w.content, w.view = max(content, 0), max(view, 0)
	w.clamp()
}

// Reset puts the cursor, top and horizontal offset back at zero.
func (w *Window) Reset() { w.cursor, w.top, w.x = 0, 0, 0 }

// Restore sets a saved position and clamps it to the current bounds.
func (w *Window) Restore(cursor, top, x int) {
	w.cursor, w.top, w.x = cursor, top, x
	w.clamp()
}

// Cursor returns the cursor row.
func (w *Window) Cursor() int { return w.cursor }

// Top returns the first visible row.
func (w *Window) Top() int { return w.top }

// XOffset returns the horizontal scroll in cells.
func (w *Window) HorizontalOffset() int { return w.x }

// Height returns the number of visible rows.
func (w *Window) Height() int { return w.height }

// MaxTop returns the largest valid top row.
func (w *Window) MaxTop() int { return max(w.rows-w.height, 0) }

// MaxX returns the largest valid horizontal offset.
func (w *Window) MaxHorizontalOffset() int { return max(w.content-w.view, 0) }

func (w *Window) clamp() {
	if w.rows == 0 {
		w.top, w.cursor, w.x = 0, 0, 0
		return
	}
	w.cursor = min(max(w.cursor, 0), w.rows-1)
	w.top = min(max(w.top, 0), w.MaxTop())
	w.x = min(max(w.x, 0), w.MaxHorizontalOffset())
	w.follow()
}

// follow scrolls the minimum needed to keep the cursor visible.
func (w *Window) follow() {
	if w.height <= 0 {
		return
	}
	if w.cursor < w.top {
		w.top = w.cursor
	}
	if w.cursor >= w.top+w.height {
		w.top = w.cursor - w.height + 1
	}
}

// --- cursor movement: the view follows the cursor ---

// Down moves the cursor down n rows, scrolling to keep it visible.
func (w *Window) Down(n int) { w.cursor += n; w.clamp() }

// Up moves the cursor up n rows, scrolling to keep it visible.
func (w *Window) Up(n int) { w.cursor -= n; w.clamp() }

// GotoTop moves the cursor to the first row.
func (w *Window) GotoTop() { w.cursor = 0; w.clamp() }

// GotoBottom moves the cursor to the last row.
func (w *Window) GotoBottom() { w.cursor = w.rows - 1; w.clamp() }

// GotoRow moves the cursor to row r, scrolling to keep it visible.
func (w *Window) GotoRow(r int) { w.cursor = r; w.clamp() }

// JumpTo puts the cursor on row r and r at the top of the view (as far as the
// last page allows).
func (w *Window) JumpTo(r int) { w.cursor, w.top = r, r; w.clamp() }

// --- viewport movement ---

// ScrollDown scrolls the view down n rows without moving the cursor, unless
// the cursor would leave the view, in which case it is pulled along.
func (w *Window) ScrollDown(n int) { w.scrollBy(n) }

// ScrollUp is the opposite of ScrollDown.
func (w *Window) ScrollUp(n int) { w.scrollBy(-n) }

func (w *Window) scrollBy(n int) {
	w.top = min(max(w.top+n, 0), w.MaxTop())
	w.cursor = min(max(w.cursor, w.top), max(w.top+w.height-1, w.top))
	w.clamp()
}

// HalfPageDown moves the view and the cursor down half a screen, as in vim.
func (w *Window) HalfPageDown() { w.shift(max(w.height/2, 1)) }

// HalfPageUp moves the view and the cursor up half a screen.
func (w *Window) HalfPageUp() { w.shift(-max(w.height/2, 1)) }

// PageDown moves the view and the cursor down a full screen.
func (w *Window) PageDown() { w.shift(max(w.height, 1)) }

// PageUp moves the view and the cursor up a full screen.
func (w *Window) PageUp() { w.shift(-max(w.height, 1)) }

func (w *Window) shift(n int) {
	w.top = min(max(w.top+n, 0), w.MaxTop())
	w.cursor += n
	w.clamp()
}

// --- horizontal scrolling ---

// Right shifts the content n cells to the right (reveals later text).
func (w *Window) Right(n int) { w.x += n; w.clamp() }

// Left shifts the content n cells to the left.
func (w *Window) Left(n int) { w.x -= n; w.clamp() }

// HalfRight scrolls right by half the visible width.
func (w *Window) HalfRight() { w.Right(max(w.view/2, 1)) }

// HalfLeft scrolls left by half the visible width.
func (w *Window) HalfLeft() { w.Left(max(w.view/2, 1)) }
