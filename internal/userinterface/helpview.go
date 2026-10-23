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
func renderHelp(theme theme.Theme, title string, global, view [][]key.Binding, width int) string {
	text := helpColumn(theme, "Global", global)
	value := helpColumn(theme, title, view)
	head := theme.HelpTitle.Render("Help") + theme.Muted.Render("  ? / esc to close")

	var body string
	switch {
	case value == "":
		body = text
	case lipgloss.Width(text)+lipgloss.Width(value)+4 <= width:
		body = lipgloss.JoinHorizontal(lipgloss.Top, text, "    ", value)
	default:
		body = text + "\n\n" + value
	}
	return head + "\n\n" + body
}

func helpColumn(theme theme.Theme, heading string, groups [][]key.Binding) string {
	var blocks []string
	for _, bindings := range groups {
		var lines []string
		for _, binding := range bindings {
			help := binding.Help()
			if !binding.Enabled() || help.Desc == "" {
				continue
			}
			pad := strings.Repeat(" ", max(0, helpKeyWidth-lipgloss.Width(help.Key)))
			lines = append(lines, theme.HelpKey.Render(help.Key)+pad+theme.HelpDescription.Render(help.Desc))
		}
		if len(lines) > 0 {
			blocks = append(blocks, strings.Join(lines, "\n"))
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return theme.HelpSection.Render(heading) + "\n" + strings.Join(blocks, "\n\n")
}
