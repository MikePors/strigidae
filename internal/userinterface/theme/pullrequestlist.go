package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// PRListStyles are the styles for pull request tables (the PR list view).
type PullRequestListStyles struct {
	Header           lipgloss.Style // column headings
	Selected         lipgloss.Style // the whole highlighted row
	Number           lipgloss.Style
	Author           lipgloss.Style
	Age              lipgloss.Style
	Draft            lipgloss.Style // the draft marker
	Approved         lipgloss.Style // review decision glyphs
	ChangesRequested lipgloss.Style
	ReviewRequired   lipgloss.Style
	ChecksPassing    lipgloss.Style // check status glyphs (also used by the PR workspace header)
	ChecksFailing    lipgloss.Style
	ChecksRunning    lipgloss.Style
	Comments         lipgloss.Style
}

func newPullRequestListStyles(r Roles) PullRequestListStyles {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	muted := fg(r.Muted)
	return PullRequestListStyles{
		Header:           muted.Bold(true),
		Selected:         lipgloss.NewStyle().Bold(true).Foreground(r.OnAccent).Background(r.Accent),
		Number:           fg(r.Warning),
		Author:           muted,
		Age:              muted,
		Draft:            muted.Italic(true),
		Approved:         fg(r.Success),
		ChangesRequested: fg(r.Error),
		ReviewRequired:   fg(r.Warning),
		ChecksPassing:    fg(r.Added),
		ChecksFailing:    fg(r.Removed),
		ChecksRunning:    fg(r.Warning),
		Comments:         muted,
	}
}
