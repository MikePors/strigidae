package workspace

import (
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// TestFocusedPaneStyling checks that the focused pane's frame and title use
// the theme's focused styles and the other pane the unfocused ones. It
// compares against what the theme's own styles produce, so a palette change
// cannot break it, only a change to which style a pane gets.
func TestFocusedPaneStyling(test *testing.T) {
	activeTheme := theme.New(true)
	if activeTheme.PaneBorderFocus.Render("│") == activeTheme.PaneBorder.Render("│") ||
		activeTheme.PaneTitleFocus.Render("x") == activeTheme.PaneTitle.Render("x") {
		test.Fatal("setup: the theme's focused and unfocused pane styles must render differently")
	}

	// The top edge is "╭─", the title, dashes, "╮"; the sides are "│".
	edge := func(border, title func(...string) string, name string) string {
		return border("╭─") + title(" "+name+" ")
	}
	focusedEdge := func(name string) string {
		return edge(activeTheme.PaneBorderFocus.Render, activeTheme.PaneTitleFocus.Render, name)
	}
	plainEdge := func(name string) string {
		return edge(activeTheme.PaneBorder.Render, activeTheme.PaneTitle.Render, name)
	}
	side := func(focused bool) string {
		if focused {
			return activeTheme.PaneBorderFocus.Render("│")
		}
		return activeTheme.PaneBorder.Render("│")
	}

	check := func(test *testing.T, out string, filesFocused bool) {
		test.Helper()
		lines := strings.Split(out, "\n")
		top, row := lines[headerRows], lines[headerRows+1]
		files, right := plainEdge("Files 0/5 viewed"), focusedEdge("Overview")
		if filesFocused {
			files, right = focusedEdge("Files 0/5 viewed"), plainEdge("Overview")
		}
		if !strings.Contains(top, files) {
			test.Errorf("Files pane top edge lacks the expected styles:\n got %q\nwant %q inside", top, files)
		}
		if !strings.Contains(top, right) {
			test.Errorf("right pane top edge lacks the expected styles:\n got %q\nwant %q inside", top, right)
		}
		// Each body row starts with the Files pane's left side and has the
		// right pane's left side where the Files pane ends.
		if !strings.HasPrefix(row, side(filesFocused)) {
			test.Errorf("Files pane side has the wrong style: %q", row)
		}
		if !strings.Contains(row, side(!filesFocused)) {
			test.Errorf("right pane side has the wrong style: %q", row)
		}
	}

	harness := loaded(test, 100, 24)
	harness.press("l")
	check(test, harness.render(), false)
	harness.press("h")
	check(test, harness.render(), true)
}
