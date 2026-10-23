package scroll

import "testing"

func win(rows, height, content, view int) *Window {
	var w Window
	w.SetRows(rows)
	w.SetSize(height)
	w.SetWidths(content, view)
	return &w
}

func TestVertical(t *testing.T) {
	tests := []struct {
		name                string
		rows, height        int
		do                  func(w *Window)
		wantCursor, wantTop int
	}{
		{"down", 20, 6, func(w *Window) { w.Down(3) }, 3, 0},
		{"up clamps at 0", 20, 6, func(w *Window) { w.Up(5) }, 0, 0},
		{"down follows with scroll", 20, 6, func(w *Window) { w.Down(8) }, 8, 3},
		{"up follows with scroll", 20, 6, func(w *Window) { w.Down(10); w.GotoTop(); w.Down(8); w.Up(7) }, 1, 1},
		{"down past end", 20, 6, func(w *Window) { w.Down(1000) }, 19, 14},
		{"goto bottom", 20, 6, func(w *Window) { w.GotoBottom() }, 19, 14},
		{"goto top", 20, 6, func(w *Window) { w.GotoBottom(); w.GotoTop() }, 0, 0},
		{"half page", 20, 6, func(w *Window) { w.HalfPageDown() }, 3, 3},
		{"half page up clamps", 20, 6, func(w *Window) { w.HalfPageUp() }, 0, 0},
		{"page down", 20, 6, func(w *Window) { w.PageDown() }, 6, 6},
		{"page down then up", 20, 6, func(w *Window) { w.PageDown(); w.PageUp() }, 0, 0},
		{"page down past end", 20, 6, func(w *Window) { w.PageDown(); w.PageDown(); w.PageDown(); w.PageDown() }, 19, 14},
		{"scroll pulls cursor along", 20, 6, func(w *Window) { w.ScrollDown(4) }, 4, 4},
		{"scroll keeps visible cursor", 20, 6, func(w *Window) { w.Down(5); w.ScrollDown(2) }, 5, 2},
		{"scroll up pulls cursor along", 20, 6, func(w *Window) { w.GotoBottom(); w.ScrollUp(3) }, 16, 11},
		{"scroll down clamps", 20, 6, func(w *Window) { w.ScrollDown(1000) }, 14, 14},
		{"scroll up clamps", 20, 6, func(w *Window) { w.Down(8); w.ScrollUp(1000) }, 5, 0},
		{"jump", 20, 6, func(w *Window) { w.JumpTo(10) }, 10, 10},
		{"jump near end clamps top", 20, 6, func(w *Window) { w.JumpTo(18) }, 18, 14},
		{"jump out of range", 20, 6, func(w *Window) { w.JumpTo(500) }, 19, 14},
		{"jump negative", 20, 6, func(w *Window) { w.Down(9); w.JumpTo(-4) }, 0, 0},
		{"empty rows", 0, 6, func(w *Window) { w.Down(3); w.GotoBottom(); w.PageDown(); w.JumpTo(4); w.ScrollDown(2) }, 0, 0},
		{"height zero", 10, 0, func(w *Window) { w.Down(4) }, 4, 0},
		{"height zero half page", 10, 0, func(w *Window) { w.HalfPageDown() }, 1, 1},
		{"height zero scroll", 10, 0, func(w *Window) { w.ScrollDown(3) }, 3, 3},
		{"height bigger than rows", 4, 10, func(w *Window) { w.Down(2); w.PageDown() }, 3, 0},
		{"height bigger than rows scroll", 4, 10, func(w *Window) { w.Down(2); w.ScrollDown(5) }, 2, 0},
		{"one row", 1, 3, func(w *Window) { w.Down(5); w.Up(9); w.GotoBottom() }, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := win(tt.rows, tt.height, 0, 0)
			tt.do(w)
			if w.Cursor() != tt.wantCursor || w.Top() != tt.wantTop {
				t.Errorf("cursor/top = %d/%d, want %d/%d", w.Cursor(), w.Top(), tt.wantCursor, tt.wantTop)
			}
		})
	}
}

func TestHorizontal(t *testing.T) {
	tests := []struct {
		name          string
		content, view int
		do            func(w *Window)
		want          int
	}{
		{"right", 100, 40, func(w *Window) { w.Right(4) }, 4},
		{"left clamps at 0", 100, 40, func(w *Window) { w.Left(5) }, 0},
		{"right clamps at max", 100, 40, func(w *Window) { w.Right(1000) }, 60},
		{"half right", 100, 40, func(w *Window) { w.HalfRight() }, 20},
		{"half left from end", 100, 40, func(w *Window) { w.Right(1000); w.HalfLeft() }, 40},
		{"content narrower than view", 30, 40, func(w *Window) { w.Right(5); w.HalfRight() }, 0},
		{"content equals view", 40, 40, func(w *Window) { w.Right(5) }, 0},
		{"view zero", 100, 0, func(w *Window) { w.HalfRight() }, 1},
		{"no content", 0, 0, func(w *Window) { w.Right(5) }, 0},
		{"half step is at least 1", 10, 1, func(w *Window) { w.HalfRight() }, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := win(10, 5, tt.content, tt.view)
			tt.do(w)
			if w.HorizontalOffset() != tt.want {
				t.Errorf("XOffset = %d, want %d", w.HorizontalOffset(), tt.want)
			}
		})
	}
}

func TestHorizontalNeedsRows(t *testing.T) {
	w := win(0, 5, 100, 40)
	w.Right(10)
	if w.HorizontalOffset() != 0 {
		t.Errorf("empty window scrolled to %d", w.HorizontalOffset())
	}
}

func TestSettersReclamp(t *testing.T) {
	w := win(20, 6, 100, 40)
	w.Down(15)
	w.Right(50)
	w.SetRows(5)
	if w.Cursor() != 4 || w.Top() != 0 {
		t.Errorf("after fewer rows: cursor/top = %d/%d", w.Cursor(), w.Top())
	}
	w.SetWidths(60, 40)
	if w.HorizontalOffset() != 20 {
		t.Errorf("after narrower content: x = %d", w.HorizontalOffset())
	}
	w.SetRows(20)
	w.GotoBottom()
	w.SetSize(3) // shrinking keeps the cursor visible
	if w.Cursor() != 19 || w.Top() != 17 {
		t.Errorf("after shrinking: cursor/top = %d/%d", w.Cursor(), w.Top())
	}
	w.SetSize(-5)
	if w.Height() != 0 {
		t.Errorf("negative height = %d", w.Height())
	}
	w.SetRows(-1)
	if w.Cursor() != 0 || w.MaxTop() != 0 {
		t.Errorf("negative rows: cursor %d maxTop %d", w.Cursor(), w.MaxTop())
	}
}

func TestResetAndRestore(t *testing.T) {
	w := win(20, 6, 100, 40)
	w.Down(9)
	w.Right(7)
	w.Reset()
	if w.Cursor() != 0 || w.Top() != 0 || w.HorizontalOffset() != 0 {
		t.Errorf("after Reset: %d/%d/%d", w.Cursor(), w.Top(), w.HorizontalOffset())
	}
	w.Restore(12, 10, 5)
	if w.Cursor() != 12 || w.Top() != 10 || w.HorizontalOffset() != 5 {
		t.Errorf("Restore: %d/%d/%d", w.Cursor(), w.Top(), w.HorizontalOffset())
	}
	w.Restore(99, -3, 1000) // clamps; the cursor stays visible
	if w.Cursor() != 19 || w.Top() != 14 || w.HorizontalOffset() != 60 {
		t.Errorf("Restore out of range: %d/%d/%d", w.Cursor(), w.Top(), w.HorizontalOffset())
	}
	w.Restore(0, 12, 0) // a cursor above the restored top pulls the top up
	if w.Cursor() != 0 || w.Top() != 0 {
		t.Errorf("Restore cursor above top: %d/%d", w.Cursor(), w.Top())
	}
}

func TestAccessors(t *testing.T) {
	w := win(20, 6, 100, 40)
	if w.Height() != 6 || w.MaxTop() != 14 || w.MaxHorizontalOffset() != 60 {
		t.Errorf("Height/MaxTop/MaxX = %d/%d/%d", w.Height(), w.MaxTop(), w.MaxHorizontalOffset())
	}
	var zero Window
	zero.Down(3)
	zero.Right(3)
	zero.PageDown()
	if zero.Cursor() != 0 || zero.Top() != 0 || zero.HorizontalOffset() != 0 {
		t.Error("zero Window should stay at 0")
	}
}

func TestGotoRow(t *testing.T) {
	var w Window
	w.SetRows(50)
	w.SetSize(10)
	w.GotoRow(25)
	if w.Cursor() != 25 || w.Top() != 16 {
		t.Errorf("cursor/top = %d/%d, want 25/16", w.Cursor(), w.Top())
	}
	w.GotoRow(3)
	if w.Cursor() != 3 || w.Top() != 3 {
		t.Errorf("cursor/top = %d/%d, want 3/3", w.Cursor(), w.Top())
	}
	w.GotoRow(99)
	if w.Cursor() != 49 {
		t.Errorf("cursor = %d, want 49 (clamped)", w.Cursor())
	}
}
