package scroll

import "testing"

func TestScrollCursor(test *testing.T) {
	tests := []struct {
		name         string
		rows, height int
		cursor       int
		do           func(window *Window)
		wantTop      int
	}{
		{"top in the middle of a long list", 50, 10, 25, (*Window).ScrollCursorTop, 25},
		{"middle in the middle of a long list", 50, 10, 25, (*Window).ScrollCursorMiddle, 21},
		{"bottom in the middle of a long list", 50, 10, 25, (*Window).ScrollCursorBottom, 16},
		{"top near the end clamps to the last page", 50, 10, 47, (*Window).ScrollCursorTop, 40},
		{"middle near the end clamps to the last page", 50, 10, 47, (*Window).ScrollCursorMiddle, 40},
		{"bottom near the end", 50, 10, 47, (*Window).ScrollCursorBottom, 38},
		{"top near the start", 50, 10, 2, (*Window).ScrollCursorTop, 2},
		{"middle near the start clamps to zero", 50, 10, 2, (*Window).ScrollCursorMiddle, 0},
		{"bottom near the start clamps to zero", 50, 10, 2, (*Window).ScrollCursorBottom, 0},
		{"top on the last row", 50, 10, 49, (*Window).ScrollCursorTop, 40},
		{"short list does not scroll (top)", 5, 10, 3, (*Window).ScrollCursorTop, 0},
		{"short list does not scroll (middle)", 5, 10, 3, (*Window).ScrollCursorMiddle, 0},
		{"short list does not scroll (bottom)", 5, 10, 3, (*Window).ScrollCursorBottom, 0},
		{"empty list", 0, 10, 0, (*Window).ScrollCursorMiddle, 0},
		{"height zero", 10, 0, 4, (*Window).ScrollCursorBottom, 4},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			window := newWindow(testCase.rows, testCase.height, 0, 0)
			window.GotoRow(testCase.cursor)
			cursor := window.Cursor()
			testCase.do(window)
			if window.Top() != testCase.wantTop {
				test.Errorf("top = %d, want %d", window.Top(), testCase.wantTop)
			}
			if window.Cursor() != cursor {
				test.Errorf("cursor moved from %d to %d", cursor, window.Cursor())
			}
		})
	}
}
