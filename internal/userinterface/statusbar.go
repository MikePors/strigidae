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
func (statusBar statusBar) render(theme theme.Theme, width int) string {
	if width <= 0 {
		return ""
	}
	right := statusBar.renderRight(theme)
	if lipgloss.Width(right) >= width {
		right = ""
	}
	avail := width - lipgloss.Width(right)

	text := " " + strings.ReplaceAll(statusBar.text, "\n", " ")
	textWidth := ansi.StringWidth(text)
	crumbWidth := ansi.StringWidth(statusBar.crumbs) + 2
	budget := min(crumbWidth, avail)
	if statusBar.kind != statusNone && crumbWidth+textWidth > avail {
		budget = min(crumbWidth, max(avail/3, avail-textWidth))
	}
	left := theme.StatusContext.Render(fitLeft(statusBar.crumbs, budget))

	midWidth := avail - lipgloss.Width(left)
	style := theme.StatusHint
	switch statusBar.kind {
	case statusInfo:
		style = theme.StatusInfo
	case statusError:
		style = theme.StatusError
	}
	text = ansi.Truncate(text, midWidth, "…")
	pad := strings.Repeat(" ", max(0, midWidth-ansi.StringWidth(text)))
	return left + style.Render(text) + theme.StatusBar.Render(pad) + right
}

func (statusBar statusBar) renderRight(theme theme.Theme) string {
	var right string
	if statusBar.pending != "" {
		right += theme.StatusPending.Render(" " + statusBar.pending)
	}
	if statusBar.spinner != "" {
		right += theme.StatusBar.Render(" ") + statusBar.spinner
	}
	return right + theme.StatusBar.Render(" ")
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
