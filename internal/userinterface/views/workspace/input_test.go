package workspace

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFocusAndFileNavigation(test *testing.T) {
	harness := loaded(test, 100, 30)
	if harness.view.focus != paneFiles || harness.view.selection() != 0 {
		test.Fatalf("initial focus=%v sel=%d", harness.view.focus, harness.view.selection())
	}
	harness.press("l")
	if harness.view.focus != paneRight {
		test.Error("l should focus the right pane")
	}
	harness.press("h")
	if harness.view.focus != paneFiles {
		test.Error("h should focus the files pane")
	}

	harness.press("j", "j")
	if harness.view.selection() != 2 {
		test.Errorf("j j: sel = %d", harness.view.selection())
	}
	harness.press("k")
	if harness.view.selection() != 1 {
		test.Errorf("k: sel = %d", harness.view.selection())
	}
	harness.press("G")
	if harness.view.selection() != 5 {
		test.Errorf("G: sel = %d", harness.view.selection())
	}
	harness.press("j")
	if harness.view.selection() != 5 {
		test.Errorf("j past the end: sel = %d", harness.view.selection())
	}
	harness.press("g g")
	if harness.view.selection() != 0 {
		test.Errorf("g g: sel = %d", harness.view.selection())
	}
	harness.press("ctrl+f")
	if harness.view.selection() != 5 {
		test.Errorf("ctrl+f: sel = %d", harness.view.selection())
	}

	// enter in the Files pane focuses the right pane.
	harness.press("enter")
	if harness.view.focus != paneRight {
		test.Error("enter should focus the right pane")
	}

	// } and { work from either pane and never land on Overview.
	harness.press("g g") // scrolls the overview, selection unchanged
	if harness.view.selection() != 5 {
		test.Errorf("g g in the right pane moved the selection to %d", harness.view.selection())
	}
	harness.press("{")
	if harness.view.selection() != 4 {
		test.Errorf("{: sel = %d", harness.view.selection())
	}
	harness.press("h", "g g", "{")
	if harness.view.selection() != 0 {
		test.Errorf("{ from Overview should stay: sel = %d", harness.view.selection())
	}
	harness.press("}")
	if harness.view.selection() != 1 {
		test.Errorf("} from Overview: sel = %d", harness.view.selection())
	}
	harness.press("l", "}", "}", "}", "}", "}")
	if harness.view.selection() != 5 {
		test.Errorf("} clamps at the last file: sel = %d", harness.view.selection())
	}
}

func TestSectionJumps(test *testing.T) {
	harness := loaded(test, 100, 14)
	harness.press("l")
	if harness.view.overview.viewport.YOffset() != 0 {
		test.Fatal("overview should start at the top")
	}
	harness.press("]")
	checks := harness.view.overview.sections[1]
	if got := harness.view.overview.viewport.YOffset(); got != checks {
		test.Errorf("]: offset = %d, want %d", got, checks)
	}
	harness.press("[")
	if got := harness.view.overview.viewport.YOffset(); got != 0 {
		test.Errorf("[: offset = %d, want 0", got)
	}
	// Works from the Files pane as well.
	harness.press("h", "]")
	if got := harness.view.overview.viewport.YOffset(); got != checks {
		test.Errorf("] from files: offset = %d, want %d", got, checks)
	}
	// Ignored while a file is selected.
	harness.press("j", "[")
	harness.press("g g", "]")
}

func TestOverviewScrolling(test *testing.T) {
	harness := loaded(test, 100, 14)
	harness.press("l", "j")
	if harness.view.overview.viewport.YOffset() != 1 {
		test.Errorf("j: offset = %d", harness.view.overview.viewport.YOffset())
	}
	harness.press("k")
	harness.press("ctrl+d")
	if harness.view.overview.viewport.YOffset() == 0 {
		test.Error("ctrl+d should scroll")
	}
	harness.press("G")
	if !harness.view.overview.viewport.AtBottom() {
		test.Error("G should reach the bottom")
	}
	harness.press("g g")
	if !harness.view.overview.viewport.AtTop() {
		test.Error("g g should reach the top")
	}
	harness.press("ctrl+f", "ctrl+b", "ctrl+u")
}

func TestOpenInBrowser(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.press("o")
	if diff := cmp.Diff([]string{"https://example.test/acme/widgets/pull/7"}, harness.opened); diff != "" {
		test.Errorf("opened (-want +got):\n%s", diff)
	}

	secondHarness := newHarness(test, 100, 24) // not loaded yet
	secondHarness.press("o")
	if len(secondHarness.opened) != 0 || len(secondHarness.messages) == 0 {
		test.Errorf("o before load: opened=%v msgs=%v", secondHarness.opened, secondHarness.messages)
	}

	thirdHarness := loaded(test, 100, 24)
	thirdHarness.view.open = func(string) error { return errors.New("no browser") }
	thirdHarness.press("o")
	if thirdHarness.lastErr() == nil {
		test.Error("browser failure should reach the status bar")
	}
}
