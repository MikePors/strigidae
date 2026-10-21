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

	// --- PR workspace ---
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
	OverlayBorder    lipgloss.Style // title text on the frame of popups such as the file picker
	OverlayBox       lipgloss.Style // rounded frame around a popup
	StrongText       lipgloss.Style // bold body text, e.g. a PR title
	OverlayItem      lipgloss.Style // unselected row in a popup
	OverlayItemFocus lipgloss.Style // selected row in a popup
	// --- end PR workspace ---

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

	bar := lipgloss.NewStyle().Background(c(barBgLight, barBgDark)).Foreground(c(barFgLight, barFgDark))
	accent := c(violetLight, violetDark)
	warm := c(orangeLight, orangeDark)
	muted := c(mutedLight, mutedDark)
	green := c(greenLight, greenDark)
	red := c(redLight, redDark)

	return Theme{
		StatusBar:     bar,
		StatusContext: bar.Bold(true).Foreground(c(onAccentLight, onAccentDark)).Background(accent),
		StatusPending: bar.Bold(true).Foreground(warm),
		StatusInfo:    bar.Foreground(c(greenLight, greenDark)),
		StatusError:   bar.Bold(true).Foreground(c(redLight, redDark)),
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
		Selected: lipgloss.NewStyle().Bold(true).Foreground(c(violetLight, violetPaleDark)),
		IsDark:   isDark,

		// --- PR workspace ---
		PaneBorder:       lipgloss.NewStyle().Foreground(muted),
		PaneBorderFocus:  lipgloss.NewStyle().Foreground(accent),
		PaneTitle:        lipgloss.NewStyle().Foreground(muted),
		PaneTitleFocus:   lipgloss.NewStyle().Bold(true).Foreground(c(onAccentLight, onAccentDark)).Background(accent),
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
		OverlayBox:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent),
		StrongText:       lipgloss.NewStyle().Bold(true),
		OverlayItem:      lipgloss.NewStyle(),
		OverlayItemFocus: lipgloss.NewStyle().Bold(true).Foreground(c(violetLight, violetPaleDark)).Reverse(true),
		// --- end PR workspace ---

		// --- Diff pane ---
		DiffContext:      lipgloss.NewStyle(),
		DiffAdd:          lipgloss.NewStyle().Background(c(addBgLight, addBgDark)),
		DiffDelete:       lipgloss.NewStyle().Background(c(deleteBgLight, deleteBgDark)),
		DiffCursor:       lipgloss.NewStyle().Background(c(cursorBgLight, cursorBgDark)),
		DiffGutter:       lipgloss.NewStyle().Foreground(muted),
		DiffGutterAdd:    lipgloss.NewStyle().Foreground(c(greenLight, greenDark)).Background(c(addGutterBgLight, addGutterBgDark)),
		DiffGutterDelete: lipgloss.NewStyle().Foreground(c(redLight, redDark)).Background(c(deleteGutterBgLight, deleteGutterBgDark)),
		DiffHunk:         lipgloss.NewStyle().Foreground(c(blueLight, blueDark)).Background(c(hunkBgLight, hunkBgDark)),
		DiffHunkCursor:   lipgloss.NewStyle().Bold(true).Foreground(c(blueLight, blueDark)).Background(c(hunkCursorBgLight, hunkCursorBgDark)),
		DiffNoNewline:    lipgloss.NewStyle().Italic(true).Foreground(muted),
		Syntax: Syntax{
			Keyword:  lipgloss.NewStyle().Foreground(c(purpleLight, purpleDark)),
			String:   lipgloss.NewStyle().Foreground(c(greenDeepLight, limeDark)),
			Comment:  lipgloss.NewStyle().Italic(true).Foreground(muted),
			Number:   lipgloss.NewStyle().Foreground(c(orangeLight, orangeStrongDark)),
			Function: lipgloss.NewStyle().Foreground(c(blueDeepLight, blueDark)),
			Type:     lipgloss.NewStyle().Foreground(c(tealLight, tealDark)),
			Operator: lipgloss.NewStyle().Foreground(c(goldLight, goldDark)),
		},
		// --- end diff pane ---

		PRList: newPRListStyles(c),
	}
}
