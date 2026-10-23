package userinterface

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

type statusKind int

const (
	statusNone statusKind = iota
	statusInfo
	statusError
)

// statusBar is the data for one rendering of the bottom bar.
type statusBar struct {
	crumbs  string     // left: breadcrumbs
	text    string     // middle: transient message, or idle hint if kind is statusNone
	kind    statusKind // style of text
	pending string     // right: pending key prefix
	spinner string     // right: rendered spinner, empty when idle
}

// render draws the bar exactly width columns wide on a single line.
//
// Priority when space is short: pending keys and spinner always stay, then a
// transient message; the breadcrumbs shrink last-first from the left (so the
// current view stays visible) down to a third of the free space. The idle
// hint is expendable.
func (statusBar statusBar) render(activeTheme theme.Theme, width int) string {
	if width <= 0 {
		return ""
	}
	right := statusBar.renderRight(activeTheme)
	if lipgloss.Width(right) >= width {
		right = ""
	}
	available := width - lipgloss.Width(right)

	text := " " + strings.ReplaceAll(statusBar.text, "\n", " ")
	textWidth := ansi.StringWidth(text)
	crumbWidth := ansi.StringWidth(statusBar.crumbs) + 2
	budget := min(crumbWidth, available)
	if statusBar.kind != statusNone && crumbWidth+textWidth > available {
		budget = min(crumbWidth, max(available/3, available-textWidth))
	}
	left := activeTheme.StatusContext.Render(fitLeft(statusBar.crumbs, budget))

	midWidth := available - lipgloss.Width(left)
	style := activeTheme.StatusHint
	switch statusBar.kind {
	case statusInfo:
		style = activeTheme.StatusInfo
	case statusError:
		style = activeTheme.StatusError
	}
	text = ansi.Truncate(text, midWidth, "…")
	pad := strings.Repeat(" ", max(0, midWidth-ansi.StringWidth(text)))
	return left + style.Render(text) + activeTheme.StatusBar.Render(pad) + right
}

func (statusBar statusBar) renderRight(activeTheme theme.Theme) string {
	var right string
	if statusBar.pending != "" {
		right += activeTheme.StatusPending.Render(" " + statusBar.pending)
	}
	if statusBar.spinner != "" {
		right += activeTheme.StatusBar.Render(" ") + statusBar.spinner
	}
	return right + activeTheme.StatusBar.Render(" ")
}

// fitLeft pads crumbs with a space each side and, if the result would exceed
// width, drops characters from the left, keeping the end of the trail.
func fitLeft(crumbs string, width int) string {
	if width <= 0 {
		return ""
	}
	if width < 3 {
		return strings.Repeat(" ", width)
	}
	inner := width - 2
	if width := ansi.StringWidth(crumbs); width > inner {
		crumbs = ansi.TruncateLeft(crumbs, width-(inner-1), "…")
	}
	return " " + crumbs + " "
}
