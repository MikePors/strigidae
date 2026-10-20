// Package theme holds every colour and named Lip Gloss style used by the UI.
//
// Views and components never call lipgloss.NewStyle with colours; they take a
// Theme and use its named styles. To add a style, add a field here.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is the set of named styles the UI draws with.
type Theme struct {
	// Status bar pieces.
	StatusBar     lipgloss.Style // filler between the segments
	StatusContext lipgloss.Style // left segment: breadcrumbs
	StatusPending lipgloss.Style // pending key prefix, like vim's showcmd
	StatusInfo    lipgloss.Style // transient informational message
	StatusError   lipgloss.Style // transient error message
	StatusHint    lipgloss.Style // short key hints shown when idle
	StatusCommand lipgloss.Style // command line row
	Spinner       lipgloss.Style // loading spinner glyph

	// Help overlay.
	HelpTitle   lipgloss.Style
	HelpSection lipgloss.Style
	HelpKey     lipgloss.Style
	HelpDesc    lipgloss.Style

	// General content.
	Title    lipgloss.Style // screen headings
	Normal   lipgloss.Style // plain text
	Muted    lipgloss.Style // de-emphasised text
	Selected lipgloss.Style // the highlighted row in a list

	// --- Diff pane (diffview) ---
	DiffContext      lipgloss.Style // code on unchanged lines
	DiffAdd          lipgloss.Style // code on added lines (carries the row background)
	DiffDelete       lipgloss.Style // code on deleted lines (carries the row background)
	DiffCursor       lipgloss.Style // code and gutter on the cursor row
	DiffGutter       lipgloss.Style // line numbers on unchanged lines
	DiffGutterAdd    lipgloss.Style // line numbers on added lines
	DiffGutterDelete lipgloss.Style // line numbers on deleted lines
	DiffHunk         lipgloss.Style // "@@ ... @@" header rows
	DiffHunkCursor   lipgloss.Style // header row under the cursor
	DiffNoNewline    lipgloss.Style // "\ No newline at end of file" rows
	Syntax           Syntax         // foreground-only token styles
	// --- end diff pane ---

	PRList PRListStyles // pull request tables
}

// Syntax holds the foreground-only styles for highlighted code tokens.
type Syntax struct {
	Keyword, String, Comment, Number, Function, Type, Operator lipgloss.Style
}

// Default returns the default theme for a dark terminal background.
func Default() Theme { return New(true) }

// New builds the default theme for a dark or light terminal background.
func New(isDark bool) Theme {
	ld := lipgloss.LightDark(isDark)
	c := func(light, dark string) color.Color {
		return ld(lipgloss.Color(light), lipgloss.Color(dark))
	}

	bar := lipgloss.NewStyle().Background(c("#e4e4ea", "#2b2b36")).Foreground(c("#3c3c4a", "#c9c9d6"))
	accent := c("#5f3dc4", "#b197fc")
	warm := c("#b8590a", "#ffc078")
	muted := c("#6b6b7b", "#8b8b9b")

	return Theme{
		StatusBar:     bar,
		StatusContext: bar.Bold(true).Foreground(c("#ffffff", "#1a1a22")).Background(accent),
		StatusPending: bar.Bold(true).Foreground(warm),
		StatusInfo:    bar.Foreground(c("#1f7a3d", "#8ce99a")),
		StatusError:   bar.Bold(true).Foreground(c("#c92a2a", "#ff8787")),
		StatusHint:    bar.Foreground(muted),
		StatusCommand: bar.Bold(true),
		Spinner:       bar.Foreground(accent),

		HelpTitle:   lipgloss.NewStyle().Bold(true).Foreground(accent),
		HelpSection: lipgloss.NewStyle().Bold(true).Underline(true),
		HelpKey:     lipgloss.NewStyle().Foreground(warm),
		HelpDesc:    lipgloss.NewStyle(),

		Title:    lipgloss.NewStyle().Bold(true).Foreground(accent),
		Normal:   lipgloss.NewStyle(),
		Muted:    lipgloss.NewStyle().Foreground(muted),
		Selected: lipgloss.NewStyle().Bold(true).Foreground(c("#5f3dc4", "#d0bfff")),

		// --- Diff pane ---
		DiffContext:      lipgloss.NewStyle(),
		DiffAdd:          lipgloss.NewStyle().Background(c("#e6ffec", "#12261e")),
		DiffDelete:       lipgloss.NewStyle().Background(c("#ffebe9", "#2d1517")),
		DiffCursor:       lipgloss.NewStyle().Background(c("#dde1f5", "#33364d")),
		DiffGutter:       lipgloss.NewStyle().Foreground(muted),
		DiffGutterAdd:    lipgloss.NewStyle().Foreground(c("#1f7a3d", "#8ce99a")).Background(c("#ccffd8", "#17382a")),
		DiffGutterDelete: lipgloss.NewStyle().Foreground(c("#c92a2a", "#ff8787")).Background(c("#ffd7d5", "#42191d")),
		DiffHunk:         lipgloss.NewStyle().Foreground(c("#0b6e99", "#74c0fc")).Background(c("#ddf4ff", "#1b2a3a")),
		DiffHunkCursor:   lipgloss.NewStyle().Bold(true).Foreground(c("#0b6e99", "#74c0fc")).Background(c("#c5e4f5", "#2c4560")),
		DiffNoNewline:    lipgloss.NewStyle().Italic(true).Foreground(muted),
		Syntax: Syntax{
			Keyword:  lipgloss.NewStyle().Foreground(c("#a626a4", "#da77f2")),
			String:   lipgloss.NewStyle().Foreground(c("#2f7d32", "#a9e34b")),
			Comment:  lipgloss.NewStyle().Italic(true).Foreground(muted),
			Number:   lipgloss.NewStyle().Foreground(c("#b8590a", "#ffa94d")),
			Function: lipgloss.NewStyle().Foreground(c("#1c62b9", "#74c0fc")),
			Type:     lipgloss.NewStyle().Foreground(c("#0a7f8f", "#66d9e8")),
			Operator: lipgloss.NewStyle().Foreground(c("#8a5a00", "#ffd43b")),
		},
		// --- end diff pane ---

		PRList: newPRListStyles(c),
	}
}
