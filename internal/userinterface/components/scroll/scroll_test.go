package scroll

import "testing"

func newWindow(rows, height, content, view int) *Window {
	var window Window
	window.SetRows(rows)
	window.SetSize(height)
	window.SetWidths(content, view)
	return &window
}

func TestVertical(test *testing.T) {
	tests := []struct {
		name                string
		rows, height        int
		do                  func(window *Window)
		wantCursor, wantTop int
	}{
		{"down", 20, 6, func(window *Window) { window.Down(3) }, 3, 0},
		{"up clamps at 0", 20, 6, func(window *Window) { window.Up(5) }, 0, 0},
		{"down follows with scroll", 20, 6, func(window *Window) { window.Down(8) }, 8, 3},
		{"up follows with scroll", 20, 6, func(window *Window) { window.Down(10); window.GotoTop(); window.Down(8); window.Up(7) }, 1, 1},
		{"down past end", 20, 6, func(window *Window) { window.Down(1000) }, 19, 14},
		{"goto bottom", 20, 6, func(window *Window) { window.GotoBottom() }, 19, 14},
		{"goto top", 20, 6, func(window *Window) { window.GotoBottom(); window.GotoTop() }, 0, 0},
		{"half page", 20, 6, func(window *Window) { window.HalfPageDown() }, 3, 3},
		{"half page up clamps", 20, 6, func(window *Window) { window.HalfPageUp() }, 0, 0},
		{"page down", 20, 6, func(window *Window) { window.PageDown() }, 6, 6},
		{"page down then up", 20, 6, func(window *Window) { window.PageDown(); window.PageUp() }, 0, 0},
		{"page down past end puts the last row at the top", 20, 6, func(window *Window) { window.PageDown(); window.PageDown(); window.PageDown(); window.PageDown() }, 19, 19},
		{"half page down past end stops with the last row at the top", 20, 6, func(window *Window) {
			for range 10 {
				window.HalfPageDown()
			}
		}, 19, 19},
		{"half page down from the bottom", 20, 6, func(window *Window) { window.GotoBottom(); window.HalfPageDown() }, 19, 17},
		{"half page up from past the end", 20, 6, func(window *Window) {
			window.PageDown()
			window.PageDown()
			window.PageDown()
			window.PageDown()
			window.HalfPageUp()
		}, 16, 16},
		{"scroll pulls cursor along", 20, 6, func(window *Window) { window.ScrollDown(4) }, 4, 4},
		{"scroll keeps visible cursor", 20, 6, func(window *Window) { window.Down(5); window.ScrollDown(2) }, 5, 2},
		{"scroll up pulls cursor along", 20, 6, func(window *Window) { window.GotoBottom(); window.ScrollUp(3) }, 16, 11},
		{"scroll down stops with the last row at the top", 20, 6, func(window *Window) { window.ScrollDown(1000) }, 19, 19},
		{"down does not scroll past the end", 20, 6, func(window *Window) { window.Down(1000); window.Down(1) }, 19, 14},
		{"goto bottom keeps a view scrolled past the end", 20, 6, func(window *Window) { window.ScrollDown(17); window.GotoBottom() }, 19, 17},
		{"scroll up clamps", 20, 6, func(window *Window) { window.Down(8); window.ScrollUp(1000) }, 5, 0},
		{"jump", 20, 6, func(window *Window) { window.JumpTo(10) }, 10, 10},
		{"jump near end clamps top", 20, 6, func(window *Window) { window.JumpTo(18) }, 18, 14},
		{"jump out of range", 20, 6, func(window *Window) { window.JumpTo(500) }, 19, 14},
		{"jump negative", 20, 6, func(window *Window) { window.Down(9); window.JumpTo(-4) }, 0, 0},
		{"empty rows", 0, 6, func(window *Window) {
			window.Down(3)
			window.GotoBottom()
			window.PageDown()
			window.JumpTo(4)
			window.ScrollDown(2)
		}, 0, 0},
		{"height zero", 10, 0, func(window *Window) { window.Down(4) }, 4, 0},
		{"height zero half page", 10, 0, func(window *Window) { window.HalfPageDown() }, 1, 1},
		{"height zero scroll", 10, 0, func(window *Window) { window.ScrollDown(3) }, 3, 3},
		{"height bigger than rows", 4, 10, func(window *Window) { window.Down(2); window.PageDown() }, 3, 3},
		{"height bigger than rows scroll", 4, 10, func(window *Window) { window.Down(2); window.ScrollDown(5) }, 3, 3},
		{"height bigger than rows down", 4, 10, func(window *Window) { window.GotoBottom() }, 3, 0},
		{"one row", 1, 3, func(window *Window) { window.Down(5); window.Up(9); window.GotoBottom() }, 0, 0},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			window := newWindow(testCase.rows, testCase.height, 0, 0)
			testCase.do(window)
			if window.Cursor() != testCase.wantCursor || window.Top() != testCase.wantTop {
				test.Errorf("cursor/top = %d/%d, want %d/%d", window.Cursor(), window.Top(), testCase.wantCursor, testCase.wantTop)
			}
		})
	}
}

func TestHorizontal(test *testing.T) {
	tests := []struct {
		name          string
		content, view int
		do            func(window *Window)
		want          int
	}{
		{"right", 100, 40, func(window *Window) { window.Right(4) }, 4},
		{"left clamps at 0", 100, 40, func(window *Window) { window.Left(5) }, 0},
		{"right clamps at max", 100, 40, func(window *Window) { window.Right(1000) }, 60},
		{"half right", 100, 40, func(window *Window) { window.HalfRight() }, 20},
		{"half left from end", 100, 40, func(window *Window) { window.Right(1000); window.HalfLeft() }, 40},
		{"content narrower than view", 30, 40, func(window *Window) { window.Right(5); window.HalfRight() }, 0},
		{"content equals view", 40, 40, func(window *Window) { window.Right(5) }, 0},
		{"view zero", 100, 0, func(window *Window) { window.HalfRight() }, 1},
		{"no content", 0, 0, func(window *Window) { window.Right(5) }, 0},
		{"half step is at least 1", 10, 1, func(window *Window) { window.HalfRight() }, 1},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			window := newWindow(10, 5, testCase.content, testCase.view)
			testCase.do(window)
			if window.HorizontalOffset() != testCase.want {
				test.Errorf("HorizontalOffset = %d, want %d", window.HorizontalOffset(), testCase.want)
			}
		})
	}
}

func TestHorizontalNeedsRows(test *testing.T) {
	window := newWindow(0, 5, 100, 40)
	window.Right(10)
	if window.HorizontalOffset() != 0 {
		test.Errorf("empty window scrolled to %d", window.HorizontalOffset())
	}
}

func TestSettersReclamp(test *testing.T) {
	window := newWindow(20, 6, 100, 40)
	window.Down(15)
	window.Right(50)
	window.SetRows(5)
	if window.Cursor() != 4 || window.Top() != 0 {
		test.Errorf("after fewer rows: cursor/top = %d/%d", window.Cursor(), window.Top())
	}
	window.SetWidths(60, 40)
	if window.HorizontalOffset() != 20 {
		test.Errorf("after narrower content: x = %d", window.HorizontalOffset())
	}
	window.SetRows(20)
	window.GotoBottom()
	window.SetSize(3) // shrinking keeps the cursor visible
	if window.Cursor() != 19 || window.Top() != 17 {
		test.Errorf("after shrinking: cursor/top = %d/%d", window.Cursor(), window.Top())
	}
	window.SetSize(-5)
	if window.Height() != 0 {
		test.Errorf("negative height = %d", window.Height())
	}
	window.SetRows(-1)
	if window.Cursor() != 0 || window.MaximumTop() != 0 {
		test.Errorf("negative rows: cursor %d maxTop %d", window.Cursor(), window.MaximumTop())
	}
}

func TestResetAndRestore(test *testing.T) {
	window := newWindow(20, 6, 100, 40)
	window.Down(9)
	window.Right(7)
	window.Reset()
	if window.Cursor() != 0 || window.Top() != 0 || window.HorizontalOffset() != 0 {
		test.Errorf("after Reset: %d/%d/%d", window.Cursor(), window.Top(), window.HorizontalOffset())
	}
	window.Restore(12, 10, 5)
	if window.Cursor() != 12 || window.Top() != 10 || window.HorizontalOffset() != 5 {
		test.Errorf("Restore: %d/%d/%d", window.Cursor(), window.Top(), window.HorizontalOffset())
	}
	window.Restore(99, -3, 1000) // clamps; the cursor stays visible
	if window.Cursor() != 19 || window.Top() != 14 || window.HorizontalOffset() != 60 {
		test.Errorf("Restore out of range: %d/%d/%d", window.Cursor(), window.Top(), window.HorizontalOffset())
	}
	window.Restore(0, 12, 0) // a cursor above the restored top pulls the top up
	if window.Cursor() != 0 || window.Top() != 0 {
		test.Errorf("Restore cursor above top: %d/%d", window.Cursor(), window.Top())
	}
}

func TestAccessors(test *testing.T) {
	window := newWindow(20, 6, 100, 40)
	if window.Height() != 6 || window.MaximumTop() != 19 || window.MaximumHorizontalOffset() != 60 {
		test.Errorf("Height/MaximumTop/MaximumHorizontalOffset = %d/%d/%d", window.Height(), window.MaximumTop(), window.MaximumHorizontalOffset())
	}
	var zero Window
	zero.Down(3)
	zero.Right(3)
	zero.PageDown()
	if zero.Cursor() != 0 || zero.Top() != 0 || zero.HorizontalOffset() != 0 {
		test.Error("zero Window should stay at 0")
	}
}

func TestGotoRow(test *testing.T) {
	var window Window
	window.SetRows(50)
	window.SetSize(10)
	window.GotoRow(25)
	if window.Cursor() != 25 || window.Top() != 16 {
		test.Errorf("cursor/top = %d/%d, want 25/16", window.Cursor(), window.Top())
	}
	window.GotoRow(3)
	if window.Cursor() != 3 || window.Top() != 3 {
		test.Errorf("cursor/top = %d/%d, want 3/3", window.Cursor(), window.Top())
	}
	window.GotoRow(99)
	if window.Cursor() != 49 {
		test.Errorf("cursor = %d, want 49 (clamped)", window.Cursor())
	}
}

// TestPastTheEnd: a view scrolled past the end stays there through setters
// that keep the rows, and comes back to the last full page when the rows
// drop or a position is restored.
func TestPastTheEnd(test *testing.T) {
	pastTheEnd := func() *Window {
		window := newWindow(20, 6, 100, 40)
		window.GotoBottom()
		window.ScrollCursorTop()
		return window
	}
	tests := []struct {
		name                string
		do                  func(window *Window)
		wantCursor, wantTop int
	}{
		{"scrolled", func(*Window) {}, 19, 19},
		{"same rows again", func(window *Window) { window.SetRows(20) }, 19, 19},
		{"more rows", func(window *Window) { window.SetRows(30) }, 19, 19},
		{"a new height", func(window *Window) { window.SetSize(8) }, 19, 19},
		{"new widths", func(window *Window) { window.SetWidths(80, 40) }, 19, 19},
		{"new row heights", func(window *Window) { window.SetRowHeight(func(int) int { return 1 }) }, 19, 19},
		{"fewer rows", func(window *Window) { window.SetRows(18) }, 17, 12},
		{"restore", func(window *Window) { window.Restore(19, 19, 0) }, 19, 14},
		{"a taller view", func(window *Window) { window.SetSize(40) }, 19, 19},
		{"scroll up", func(window *Window) { window.ScrollUp(2) }, 19, 17},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			window := pastTheEnd()
			testCase.do(window)
			if window.Cursor() != testCase.wantCursor || window.Top() != testCase.wantTop {
				test.Errorf("cursor/top = %d/%d, want %d/%d", window.Cursor(), window.Top(), testCase.wantCursor, testCase.wantTop)
			}
		})
	}
}

// TestLastPageFollowsResize: a view on the last full page, not past it,
// stays on the last full page when the view grows or the rows get shorter,
// so the rows above come into view instead of empty space.
func TestLastPageFollowsResize(test *testing.T) {
	tests := []struct {
		name    string
		rows    int
		height  int
		heights func(row int) int
		do      func(window *Window)
		wantTop int
	}{
		{"a taller view", 30, 10, nil, func(window *Window) { window.SetSize(40) }, 0},
		{"a little taller", 30, 10, nil, func(window *Window) { window.SetSize(12) }, 18},
		{"shorter rows", 20, 10, func(int) int { return 2 }, func(window *Window) { window.SetRowHeight(nil) }, 10},
		{"more rows keep the top", 30, 10, nil, func(window *Window) { window.SetRows(40) }, 20},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			window := newWindow(testCase.rows, testCase.height, 0, 0)
			window.SetRowHeight(testCase.heights)
			window.GotoBottom()
			testCase.do(window)
			if window.Top() != testCase.wantTop {
				test.Errorf("top = %d, want %d", window.Top(), testCase.wantTop)
			}
		})
	}
}
