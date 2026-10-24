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
	harness := loaded(test, 100, 9)
	harness.press("G")
	harness.view.Render(100, 9)
	cursor := harness.view.selection()
	for _, key := range []string{"z t", "z z", "z b"} {
		harness.press(key)
		if got := harness.view.selection(); got != cursor {
			test.Errorf("%s moved the selection from %d to %d", key, cursor, got)
		}
		if got := harness.view.window.Cursor(); got != cursor {
			test.Errorf("%s moved the cursor to %d", key, got)
		}
	}
}
