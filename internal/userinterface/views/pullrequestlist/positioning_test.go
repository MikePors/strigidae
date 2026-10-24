package pullrequestlist

import "testing"

func TestScreenPositioningMovesTheViewNotTheCursor(test *testing.T) {
	tests := []struct {
		key     string
		wantTop int
	}{
		{"z t", 10},
		{"z z", 5},
		{"z b", 0},
	}
	for _, testCase := range tests {
		test.Run(testCase.key, func(test *testing.T) {
			view := loadedView(test, fakeWith(makePullRequests(25)), nil) // window shows 11 rows
			for range 10 {
				press(view, "j")
			}
			press(view, testCase.key)
			if got := view.window.Cursor(); got != 10 {
				test.Errorf("cursor = %d, want 10", got)
			}
			if got := view.window.Top(); got != testCase.wantTop {
				test.Errorf("top = %d, want %d", got, testCase.wantTop)
			}
		})
	}
}
