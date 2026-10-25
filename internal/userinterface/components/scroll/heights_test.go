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
		{"z t is limited by the last page", 4, []int{1, 1, 1, 2, 1}, func(window *Window) { window.Down(3); window.ScrollCursorTop() }, 3, 2},
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
	if got := window.MaximumTop(); got != 5 {
		test.Errorf("MaximumTop = %d, want 5", got)
	}
	if got := window.LastVisible(); got != 1 {
		test.Errorf("LastVisible = %d, want 1 (rows 0 and 1 fill 4 screen rows)", got)
	}
	tall := heightWindow(4, 1, 6, 1)
	tall.Down(1)
	if got := tall.LastVisible(); got != 1 {
		test.Errorf("LastVisible on a row taller than the view = %d, want the row itself", got)
	}
	tall.SetRowHeight(nil)
	if got := tall.MaximumTop(); got != 0 {
		test.Errorf("after SetRowHeight(nil), MaximumTop = %d, want 0 (three rows fit)", got)
	}
}
