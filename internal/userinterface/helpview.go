package userinterface

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

const helpKeyWidth = 9

// renderHelp lays out the help overlay: global bindings on the left and the
// active view's on the right (stacked when the terminal is narrow).
func renderHelp(activeTheme theme.Theme, title string, global, viewBindings [][]key.Binding, width int) string {
	globalColumn := helpColumn(activeTheme, "Global", global)
	viewColumn := helpColumn(activeTheme, title, viewBindings)
	head := activeTheme.HelpTitle.Render("Help") + activeTheme.Muted.Render("  ? / esc to close")

	var body string
	switch {
	case viewColumn == "":
		body = globalColumn
	case lipgloss.Width(globalColumn)+lipgloss.Width(viewColumn)+4 <= width:
		body = lipgloss.JoinHorizontal(lipgloss.Top, globalColumn, "    ", viewColumn)
	default:
		body = globalColumn + "\n\n" + viewColumn
	}
	return head + "\n\n" + body
}

func helpColumn(activeTheme theme.Theme, heading string, groups [][]key.Binding) string {
	var blocks []string
	for _, bindings := range groups {
		var lines []string
		for _, binding := range bindings {
			help := binding.Help()
			if !binding.Enabled() || help.Desc == "" {
				continue
			}
			pad := strings.Repeat(" ", max(0, helpKeyWidth-lipgloss.Width(help.Key)))
			lines = append(lines, activeTheme.HelpKey.Render(help.Key)+pad+activeTheme.HelpDescription.Render(help.Desc))
		}
		if len(lines) > 0 {
			blocks = append(blocks, strings.Join(lines, "\n"))
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return activeTheme.HelpSection.Render(heading) + "\n" + strings.Join(blocks, "\n\n")
}
