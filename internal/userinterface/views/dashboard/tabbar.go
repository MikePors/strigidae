package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

const tabGap = " "

// tabBar draws the tab names with the active one highlighted. When they do
// not all fit, tabs before the active one are dropped from the left.
func (view *View) tabBar(width int) string {
	chips := make([]string, len(view.tabs))
	widths := make([]int, len(view.tabs))
	for i, current := range view.tabs {
		label := " " + terminaltext.Clean(current.name) + " "
		widths[i] = ansi.StringWidth(label)
		if i == view.active {
			chips[i] = view.theme.Dashboard.ActiveTab.Render(label)
		} else {
			chips[i] = view.theme.Dashboard.InactiveTab.Render(label)
		}
	}
	first := 0
	for first < view.active && sum(widths[first:view.active+1])+len(tabGap)*(view.active-first) > width {
		first++
	}
	return ansi.Truncate(strings.Join(chips[first:], tabGap), width, "")
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
