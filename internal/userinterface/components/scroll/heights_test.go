package scroll

import "testing"

// heightWindow is a window over rows whose screen heights are given.
func heightWindow(height int, heights ...int) *Window {
	window := newWindow(len(heights), height, 0, 0)
	window.SetRowHeight(func(row int) int { return heights[row] })
	return window
}

func TestRowHeights(test *testing.T) {
	tests := []struct {
		name                string
		height              int
		heights             []int
		do                  func(window *Window)
		wantCursor, wantTop int
	}{
		{"a tall row that fits keeps the top", 4, []int{1, 3, 1, 1, 2, 1, 1, 1}, func(window *Window) { window.Down(1) }, 1, 0},
		{"the next row scrolls the whole tall row's worth", 4, []int{1, 3, 1, 1, 2, 1, 1, 1}, func(window *Window) { window.Down(2) }, 2, 1},
		{"bottom fills the view with the last rows", 4, []int{1, 3, 1, 1, 2, 1, 1, 1}, func(window *Window) { window.GotoBottom() }, 7, 5},
		{"a row taller than the view sits at the top", 4, []int{1, 6, 1}, func(window *Window) { window.Down(1) }, 1, 1},
		{"page down moves a screen of rows", 6, []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2}, func(window *Window) { window.PageDown() }, 3, 3},
		{"scrolling one screen row passes a tall top row", 4, []int{3, 1, 1, 1, 1, 1}, func(window *Window) { window.ScrollDown(1) }, 1, 1},
		{"scrolling pulls the cursor to the first visible row", 4, []int{1, 3, 1, 1, 1, 1}, func(window *Window) { window.ScrollDown(1) }, 1, 1},
		{"z t scrolls past the end", 4, []int{1, 1, 1, 2, 1}, func(window *Window) { window.Down(3); window.ScrollCursorTop() }, 3, 3},
		{"z t on the last row puts it at the top", 4, []int{1, 1, 1, 2, 1}, func(window *Window) { window.GotoBottom(); window.ScrollCursorTop() }, 4, 4},
		{"page down stops with the last row at the top", 4, []int{1, 3, 1, 1, 2, 1, 1, 1}, func(window *Window) {
			for range 10 {
				window.PageDown()
			}
		}, 7, 7},
		{"jumping near the end stops at the last page", 4, []int{1, 3, 1, 1, 2, 1, 1, 1}, func(window *Window) { window.JumpTo(6) }, 6, 5},
		{"z b puts the whole cursor row at the bottom", 4, []int{1, 1, 1, 2, 1}, func(window *Window) { window.Down(3); window.ScrollCursorBottom() }, 3, 1},
		{"z z centres the cursor row", 4, []int{1, 1, 1, 2, 1}, func(window *Window) { window.Down(3); window.ScrollCursorMiddle() }, 3, 2},
		{"z z centres a tall cursor row by its height", 7, []int{1, 1, 1, 1, 3, 1, 1, 1, 1}, func(window *Window) { window.Down(4); window.ScrollCursorMiddle() }, 4, 2},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			window := heightWindow(testCase.height, testCase.heights...)
			testCase.do(window)
			if window.Cursor() != testCase.wantCursor || window.Top() != testCase.wantTop {
				test.Errorf("cursor %d top %d, want cursor %d top %d", window.Cursor(), window.Top(), testCase.wantCursor, testCase.wantTop)
			}
		})
	}
}

func TestRowHeightBounds(test *testing.T) {
	window := heightWindow(4, 1, 3, 1, 1, 2, 1, 1, 1)
	if got := window.MaximumTop(); got != 7 {
		test.Errorf("MaximumTop = %d, want 7 (the last row)", got)
	}
	if got := window.lastPageTop(); got != 5 {
		test.Errorf("lastPageTop = %d, want 5", got)
	}
	if got := window.lastVisible(window.Top()); got != 1 {
		test.Errorf("lastVisible = %d, want 1 (rows 0 and 1 fill 4 screen rows)", got)
	}
	tall := heightWindow(4, 1, 6, 1)
	tall.Down(1)
	if got := tall.lastVisible(tall.Top()); got != 1 {
		test.Errorf("lastVisible on a row taller than the view = %d, want the row itself", got)
	}
	tall.SetRowHeight(nil)
	if got := tall.lastPageTop(); got != 0 {
		test.Errorf("after SetRowHeight(nil), lastPageTop = %d, want 0 (three rows fit)", got)
	}
}

// TestRowHeightAskedOnlyInRange: no operation asks the callback about a row
// that does not exist, including on an empty window.
func TestRowHeightAskedOnlyInRange(test *testing.T) {
	for _, rows := range []int{0, 1, 5} {
		window := newWindow(rows, 3, 0, 0)
		window.SetRowHeight(func(row int) int {
			if row < 0 || row >= rows {
				test.Fatalf("asked for row %d of %d", row, rows)
			}
			return 2
		})
		for _, operation := range []func(){
			window.ScrollCursorTop, window.ScrollCursorMiddle, window.ScrollCursorBottom,
			window.PageDown, window.PageUp, window.HalfPageDown, window.HalfPageUp,
			window.GotoBottom, window.GotoTop, func() { window.ScrollDown(2) }, func() { window.ScrollUp(2) },
			func() { window.Down(3) }, func() { window.Up(3) }, func() { window.JumpTo(rows) },
		} {
			operation()
		}
	}
}
