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

	// IsDark reports whether this is the dark variant; renderers that need a
	// fixed palette (Glamour) pick theirs from it instead of auto-detecting.
	IsDark bool

	// --- PR workspace (#7) ---
	PaneBorder       lipgloss.Style // frame of an unfocused pane
	PaneBorderFocus  lipgloss.Style // frame of the focused pane
	PaneTitle        lipgloss.Style // title in an unfocused pane's top edge
	PaneTitleFocus   lipgloss.Style // title in the focused pane's top edge
	FileAdded        lipgloss.Style // change-type marker and +N counts
	FileDeleted      lipgloss.Style // change-type marker and -N counts
	FileModified     lipgloss.Style // change-type marker for modified, renamed and other files
	StateOpen        lipgloss.Style // PR state badges
	StateDraft       lipgloss.Style
	StateMerged      lipgloss.Style
	StateClosed      lipgloss.Style
	Author           lipgloss.Style // comment and PR authors
	Pending          lipgloss.Style // pending (draft) review marker
	ErrorText        lipgloss.Style // inline error messages in content areas
	SectionHeading   lipgloss.Style // "Description" / "Conversation" headings
	OverlayBorder    lipgloss.Style // frame of popups such as the file picker
	OverlayItem      lipgloss.Style // unselected row in a popup
	OverlayItemFocus lipgloss.Style // selected row in a popup
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
	green := c("#1f7a3d", "#8ce99a")
	red := c("#c92a2a", "#ff8787")

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
		IsDark:   isDark,

		PaneBorder:       lipgloss.NewStyle().Foreground(muted),
		PaneBorderFocus:  lipgloss.NewStyle().Foreground(accent),
		PaneTitle:        lipgloss.NewStyle().Foreground(muted),
		PaneTitleFocus:   lipgloss.NewStyle().Bold(true).Foreground(c("#ffffff", "#1a1a22")).Background(accent),
		FileAdded:        lipgloss.NewStyle().Foreground(green),
		FileDeleted:      lipgloss.NewStyle().Foreground(red),
		FileModified:     lipgloss.NewStyle().Foreground(warm),
		StateOpen:        lipgloss.NewStyle().Bold(true).Foreground(green),
		StateDraft:       lipgloss.NewStyle().Bold(true).Foreground(muted),
		StateMerged:      lipgloss.NewStyle().Bold(true).Foreground(accent),
		StateClosed:      lipgloss.NewStyle().Bold(true).Foreground(red),
		Author:           lipgloss.NewStyle().Bold(true),
		Pending:          lipgloss.NewStyle().Bold(true).Foreground(warm),
		ErrorText:        lipgloss.NewStyle().Bold(true).Foreground(red),
		SectionHeading:   lipgloss.NewStyle().Bold(true).Foreground(accent),
		OverlayBorder:    lipgloss.NewStyle().Foreground(accent),
		OverlayItem:      lipgloss.NewStyle(),
		OverlayItemFocus: lipgloss.NewStyle().Bold(true).Foreground(c("#5f3dc4", "#d0bfff")).Reverse(true),
	}
}
