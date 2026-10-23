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
func (s statusBar) render(th theme.Theme, width int) string {
	if width <= 0 {
		return ""
	}
	right := s.renderRight(th)
	if lipgloss.Width(right) >= width {
		right = ""
	}
	avail := width - lipgloss.Width(right)

	text := " " + strings.ReplaceAll(s.text, "\n", " ")
	textW := ansi.StringWidth(text)
	crumbW := ansi.StringWidth(s.crumbs) + 2
	budget := min(crumbW, avail)
	if s.kind != statusNone && crumbW+textW > avail {
		budget = min(crumbW, max(avail/3, avail-textW))
	}
	left := th.StatusContext.Render(fitLeft(s.crumbs, budget))

	midW := avail - lipgloss.Width(left)
	style := th.StatusHint
	switch s.kind {
	case statusInfo:
		style = th.StatusInfo
	case statusError:
		style = th.StatusError
	}
	text = ansi.Truncate(text, midW, "…")
	pad := strings.Repeat(" ", max(0, midW-ansi.StringWidth(text)))
	return left + style.Render(text) + th.StatusBar.Render(pad) + right
}

func (s statusBar) renderRight(th theme.Theme) string {
	var right string
	if s.pending != "" {
		right += th.StatusPending.Render(" " + s.pending)
	}
	if s.spinner != "" {
		right += th.StatusBar.Render(" ") + s.spinner
	}
	return right + th.StatusBar.Render(" ")
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
	if w := ansi.StringWidth(crumbs); w > inner {
		crumbs = ansi.TruncateLeft(crumbs, w-(inner-1), "…")
	}
	return " " + crumbs + " "
}
