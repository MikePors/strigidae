package pullrequestlist

import "testing"

func TestScreenPositioningMovesTheViewNotTheCursor(test *testing.T) {
	tests := []struct {
		key     string
		wantTop int
	}{
		{"z t", 8},
		{"z z", 3},
		{"z b", 0},
	}
	for _, testCase := range tests {
		test.Run(testCase.key, func(test *testing.T) {
			view := loadedView(test, fakeWith(makePullRequests(25)), nil) // window shows 11 rows
			for range 12 {
				press(view, "j")
			}
			for range 4 {
				press(view, "k")
			}
			// Cursor 8 in a view that starts at row 2: every key must move the view.
			if view.current().window.Cursor() != 8 || view.current().window.Top() != 2 {
				test.Fatalf("start: cursor %d, top %d; want 8, 2", view.current().window.Cursor(), view.current().window.Top())
			}
			press(view, testCase.key)
			if got := view.current().window.Cursor(); got != 8 {
				test.Errorf("cursor = %d, want 8", got)
			}
			if got := view.current().window.Top(); got != testCase.wantTop {
				test.Errorf("top = %d, want %d", got, testCase.wantTop)
			}
		})
	}
}
