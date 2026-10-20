package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// PRListStyles are the styles for pull request tables (the PR list view).
type PRListStyles struct {
	Header           lipgloss.Style // column headings
	Selected         lipgloss.Style // the whole highlighted row
	Number           lipgloss.Style
	Author           lipgloss.Style
	Age              lipgloss.Style
	Draft            lipgloss.Style // the draft marker
	Approved         lipgloss.Style // review decision glyphs
	ChangesRequested lipgloss.Style
	ReviewRequired   lipgloss.Style
	Comments         lipgloss.Style
}

func newPRListStyles(c func(light, dark string) color.Color) PRListStyles {
	muted := lipgloss.NewStyle().Foreground(c("#6b6b7b", "#8b8b9b"))
	return PRListStyles{
		Header:           muted.Bold(true),
		Selected:         lipgloss.NewStyle().Bold(true).Foreground(c("#ffffff", "#1a1a22")).Background(c("#5f3dc4", "#b197fc")),
		Number:           lipgloss.NewStyle().Foreground(c("#b8590a", "#ffc078")),
		Author:           muted,
		Age:              muted,
		Draft:            muted.Italic(true),
		Approved:         lipgloss.NewStyle().Foreground(c("#1f7a3d", "#8ce99a")),
		ChangesRequested: lipgloss.NewStyle().Foreground(c("#c92a2a", "#ff8787")),
		ReviewRequired:   lipgloss.NewStyle().Foreground(c("#b8590a", "#ffc078")),
		Comments:         muted,
	}
}
