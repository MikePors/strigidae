package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/MikePors/strigidae/internal/ui/theme"
)

const helpKeyWidth = 9

// renderHelp lays out the help overlay: global bindings on the left and the
// active view's on the right (stacked when the terminal is narrow).
func renderHelp(th theme.Theme, title string, global, view [][]key.Binding, width int) string {
	g := helpColumn(th, "Global", global)
	v := helpColumn(th, title, view)
	head := th.HelpTitle.Render("Help") + th.Muted.Render("  ? / q / esc to close")

	var body string
	switch {
	case v == "":
		body = g
	case lipgloss.Width(g)+lipgloss.Width(v)+4 <= width:
		body = lipgloss.JoinHorizontal(lipgloss.Top, g, "    ", v)
	default:
		body = g + "\n\n" + v
	}
	return head + "\n\n" + body
}

func helpColumn(th theme.Theme, heading string, groups [][]key.Binding) string {
	var blocks []string
	for _, grp := range groups {
		var lines []string
		for _, b := range grp {
			h := b.Help()
			if !b.Enabled() || h.Desc == "" {
				continue
			}
			pad := strings.Repeat(" ", max(0, helpKeyWidth-lipgloss.Width(h.Key)))
			lines = append(lines, th.HelpKey.Render(h.Key)+pad+th.HelpDesc.Render(h.Desc))
		}
		if len(lines) > 0 {
			blocks = append(blocks, strings.Join(lines, "\n"))
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return th.HelpSection.Render(heading) + "\n" + strings.Join(blocks, "\n\n")
}
