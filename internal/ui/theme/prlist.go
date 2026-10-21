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
	muted := lipgloss.NewStyle().Foreground(c(mutedLight, mutedDark))
	return PRListStyles{
		Header:           muted.Bold(true),
		Selected:         lipgloss.NewStyle().Bold(true).Foreground(c(onAccentLight, onAccentDark)).Background(c(violetLight, violetDark)),
		Number:           lipgloss.NewStyle().Foreground(c(orangeLight, orangeDark)),
		Author:           muted,
		Age:              muted,
		Draft:            muted.Italic(true),
		Approved:         lipgloss.NewStyle().Foreground(c(greenLight, greenDark)),
		ChangesRequested: lipgloss.NewStyle().Foreground(c(redLight, redDark)),
		ReviewRequired:   lipgloss.NewStyle().Foreground(c(orangeLight, orangeDark)),
		Comments:         muted,
	}
}
