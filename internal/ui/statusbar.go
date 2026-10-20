package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/ui/theme"
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
func (s statusBar) render(th theme.Theme, width int) string {
	if width <= 0 {
		return ""
	}
	left := th.StatusContext.Render(" " + s.crumbs + " ")
	if lipgloss.Width(left) > width {
		return th.StatusContext.Render(ansi.Truncate(left, width, "…"))
	}

	var right string
	if s.pending != "" {
		right += th.StatusPending.Render(" " + s.pending)
	}
	if s.spinner != "" {
		right += th.StatusBar.Render(" ") + s.spinner
	}
	right += th.StatusBar.Render(" ")
	if lipgloss.Width(left)+lipgloss.Width(right) > width {
		right = ""
	}

	midW := width - lipgloss.Width(left) - lipgloss.Width(right)
	style := th.StatusHint
	switch s.kind {
	case statusInfo:
		style = th.StatusInfo
	case statusError:
		style = th.StatusError
	}
	text := ansi.Truncate(" "+strings.ReplaceAll(s.text, "\n", " "), midW, "…")
	mid := style.Render(text) + th.StatusBar.Render(strings.Repeat(" ", max(0, midW-ansi.StringWidth(text))))
	return left + mid + right
}
