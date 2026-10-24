package workspace

import "testing"

func TestScreenPositioningInTheDiffPane(test *testing.T) {
	harness := openClientDiff(test, 100, 14)
	harness.press("j", "j", "j", "j", "j", "j", "j", "j", "j", "j")
	harness.view.Render(100, 14)
	pane := harness.pane()
	cursor := pane.CursorRow()
	check := func(key string) int {
		harness.press(key)
		harness.view.Render(100, 14)
		if got := pane.CursorRow(); got != cursor {
			test.Errorf("%s moved the cursor from %d to %d", key, cursor, got)
		}
		return cursor - pane.Top() // rows above the cursor in the view
	}
	height := check("z b") + 1
	if height < 3 || height > cursor+1 {
		test.Fatalf("z b put the cursor %d rows from the top; the view looks unscrolled", height-1)
	}
	if got, want := check("z z"), (height-1)/2; got != want {
		test.Errorf("z z: %d rows above the cursor, want %d", got, want)
	}
	if got := check("z t"); got != 0 {
		test.Errorf("z t: %d rows above the cursor, want 0", got)
	}
}

func TestScreenPositioningInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, headerRows+2+3) // a three-row Files list over Overview and five files
	harness.view.Render(100, headerRows+2+3)
	if got, want := harness.view.window.Height(), 3; got != want {
		test.Fatalf("Files list height = %d, want %d", got, want)
	}
	harness.press("j", "j", "j") // the cursor is on row 3 of 6
	tests := []struct {
		key     string
		wantTop int
	}{{"z t", 3}, {"z z", 2}, {"z b", 1}}
	for _, testCase := range tests {
		harness.press(testCase.key)
		if got := harness.view.window.Cursor(); got != 3 {
			test.Errorf("%s moved the cursor to %d", testCase.key, got)
		}
		if got := harness.view.window.Top(); got != testCase.wantTop {
			test.Errorf("%s: top = %d, want %d", testCase.key, got, testCase.wantTop)
		}
	}
}
