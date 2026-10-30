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

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
