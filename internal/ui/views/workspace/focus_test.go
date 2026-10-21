package workspace

import (
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/ui/theme"
)

// TestFocusedPaneStyling checks that the focused pane's frame and title use
// the theme's focused styles and the other pane the unfocused ones. It
// compares against what the theme's own styles produce, so a palette change
// cannot break it, only a change to which style a pane gets.
func TestFocusedPaneStyling(t *testing.T) {
	th := theme.New(true)
	if th.PaneBorderFocus.Render("│") == th.PaneBorder.Render("│") ||
		th.PaneTitleFocus.Render("x") == th.PaneTitle.Render("x") {
		t.Fatal("setup: the theme's focused and unfocused pane styles must render differently")
	}

	// The top edge is "╭─", the title, dashes, "╮"; the sides are "│".
	edge := func(border, title func(...string) string, name string) string {
		return border("╭─") + title(" "+name+" ")
	}
	focusedEdge := func(name string) string {
		return edge(th.PaneBorderFocus.Render, th.PaneTitleFocus.Render, name)
	}
	plainEdge := func(name string) string {
		return edge(th.PaneBorder.Render, th.PaneTitle.Render, name)
	}
	side := func(focused bool) string {
		if focused {
			return th.PaneBorderFocus.Render("│")
		}
		return th.PaneBorder.Render("│")
	}

	check := func(t *testing.T, out string, filesFocused bool) {
		t.Helper()
		lines := strings.Split(out, "\n")
		top, row := lines[headerRows], lines[headerRows+1]
		files, right := plainEdge("Files"), focusedEdge("Overview")
		if filesFocused {
			files, right = focusedEdge("Files"), plainEdge("Overview")
		}
		if !strings.Contains(top, files) {
			t.Errorf("Files pane top edge lacks the expected styles:\n got %q\nwant %q inside", top, files)
		}
		if !strings.Contains(top, right) {
			t.Errorf("right pane top edge lacks the expected styles:\n got %q\nwant %q inside", top, right)
		}
		// Each body row starts with the Files pane's left side and has the
		// right pane's left side where the Files pane ends.
		if !strings.HasPrefix(row, side(filesFocused)) {
			t.Errorf("Files pane side has the wrong style: %q", row)
		}
		if !strings.Contains(row, side(!filesFocused)) {
			t.Errorf("right pane side has the wrong style: %q", row)
		}
	}

	h := loaded(t, 100, 24)
	h.press("l")
	check(t, h.render(), false)
	h.press("h")
	check(t, h.render(), true)
}
