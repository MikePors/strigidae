// Package tabbar draws the row of tab names shared by the dashboard and the
// repository pull request list.
package tabbar

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

const gap = " "

// Render draws the tab names with the active one highlighted. When they do
// not all fit, tabs before the active one are dropped from the left.
func Render(activeTheme theme.Theme, names []string, active, width int) string {
	chips := make([]string, len(names))
	widths := make([]int, len(names))
	for i, name := range names {
		label := " " + terminaltext.Clean(name) + " "
		widths[i] = ansi.StringWidth(label)
		if i == active {
			chips[i] = activeTheme.Dashboard.ActiveTab.Render(label)
		} else {
			chips[i] = activeTheme.Dashboard.InactiveTab.Render(label)
		}
	}
	first := 0
	for first < active && sum(widths[first:active+1])+len(gap)*(active-first) > width {
		first++
	}
	return ansi.Truncate(strings.Join(chips[first:], gap), width, "")
}

// RenderWithNote is Render with a note at the right end of the line, such as
// the list's sort. The note is drawn only while it takes at most half the
// width; the tabs get what is left. note must be clean.
func RenderWithNote(activeTheme theme.Theme, names []string, active, width int, note string) string {
	noteWidth := ansi.StringWidth(note)
	if note == "" || 2*(noteWidth+len(gap)) > width {
		return Render(activeTheme, names, active, width)
	}
	tabsWidth := width - noteWidth - len(gap)
	tabs := Render(activeTheme, names, active, tabsWidth)
	padding := strings.Repeat(" ", tabsWidth-ansi.StringWidth(tabs)+len(gap))
	return tabs + padding + activeTheme.Muted.Render(note)
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
