// Package theme holds every colour and named Lip Gloss style used by the UI.
//
// Colour flows through three layers, each built only from the one below:
//
//  1. A Palette: the 24 slots of Base24 plus a light/dark Variant. Palettes
//     are the only place hex values exist (palettes.go); FromBase16 and
//     FromANSI map other colour schemes into one.
//  2. Roles: what strig means by a colour (text, muted, accent, added,
//     tints, syntax tokens, ...). deriveRoles in roles.go is the only code
//     that reads palette slots. A theme may override any role with an exact
//     colour, used as written. A contrast guard nudges derived foregrounds
//     that would be unreadable; a failing override is only logged.
//  3. Styles: the named Lip Gloss styles on Theme, built from roles only.
//
// Views and components never call lipgloss.NewStyle with colours; they take a
// Theme and use its named styles. To add a style, add a field here and build
// it from an existing role; if no role fits, add one with a default
// derivation. Syntax highlighting colours follow the same path: a Chroma
// style is generated from the syntax roles.
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
	CommandPrompt lipgloss.Style // the ":" before typed text
	CommandText   lipgloss.Style // text typed on the command line, and its cursor
	CommandHint   lipgloss.Style // placeholder and completion hint on the command line
	Spinner       lipgloss.Style // loading spinner glyph

	HeadingMarker lipgloss.Style // the "#" marks before Markdown headings

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

	// IsDark reports whether the palette is the dark variant.
	IsDark bool
	// ID fingerprints the final roles and the variant, so any change to a
	// role, including an override, changes it. Renderer caches include it.
	ID string
	// Roles are the colours the styles were built from. Renderers with their
	// own style systems (Markdown) read them; views use the named styles.
	Roles Roles

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

// Default returns the default theme for a dark terminal background.
func Default() Theme { return FromPalette(builtinDark) }

// New builds the built-in theme for a dark or light terminal background.
func New(isDark bool) Theme {
	if isDark {
		return FromPalette(builtinDark)
	}
	return FromPalette(builtinLight)
}

// FromPalette builds a theme from a palette with default roles.
func FromPalette(p Palette) Theme { return FromPaletteWithOverrides(p, Roles{}) }

// FromPaletteWithOverrides builds a theme from a palette, replacing every
// role that ov sets (non-nil fields) with its exact colour.
func FromPaletteWithOverrides(p Palette, ov Roles) Theme {
	return newTheme(p, deriveRoles(p, ov))
}

func newTheme(p Palette, r Roles) Theme {
	bar := lipgloss.NewStyle().Background(r.Surface).Foreground(r.Text)
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	bold := func(c color.Color) lipgloss.Style { return fg(c).Bold(true) }

	return Theme{
		StatusBar:     bar,
		StatusContext: bar.Bold(true).Foreground(r.OnAccent).Background(r.Accent),
		StatusPending: bar.Bold(true).Foreground(r.Warning),
		StatusInfo:    bar.Foreground(r.Success),
		StatusError:   bar.Bold(true).Foreground(r.Error),
		StatusHint:    bar.Foreground(r.Muted),
		StatusCommand: bar.Bold(true),
		CommandPrompt: bar.Bold(true).Foreground(r.Accent),
		CommandText:   bar.Foreground(r.Text),
		CommandHint:   bar.Foreground(r.Muted),
		Spinner:       bar.Foreground(r.Accent),

		HeadingMarker: fg(r.Muted),

		HelpTitle:   bold(r.Accent),
		HelpSection: lipgloss.NewStyle().Bold(true).Underline(true),
		HelpKey:     fg(r.Warning),
		HelpDesc:    lipgloss.NewStyle(),

		Title:    bold(r.Accent),
		Normal:   lipgloss.NewStyle(),
		Muted:    fg(r.Muted),
		Selected: bold(r.AccentStrong),
		IsDark:   p.Variant == Dark,
		ID:       r.fingerprint(p.Variant, ""),
		Roles:    r,

		// --- PR workspace ---
		PaneBorder:       fg(r.Muted),
		PaneBorderFocus:  fg(r.Accent),
		PaneTitle:        fg(r.Muted),
		PaneTitleFocus:   bold(r.OnAccent).Background(r.Accent),
		FileAdded:        fg(r.Added),
		FileDeleted:      fg(r.Removed),
		FileModified:     fg(r.Warning),
		StateOpen:        bold(r.Success),
		StateDraft:       bold(r.Muted),
		StateMerged:      bold(r.Accent),
		StateClosed:      bold(r.Error),
		Author:           lipgloss.NewStyle().Bold(true),
		Pending:          bold(r.Warning),
		ErrorText:        bold(r.Error),
		SectionHeading:   bold(r.Accent),
		OverlayBorder:    fg(r.Accent),
		OverlayBox:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(r.Accent),
		StrongText:       lipgloss.NewStyle().Bold(true),
		OverlayItem:      lipgloss.NewStyle(),
		OverlayItemFocus: bold(r.AccentStrong).Reverse(true),
		// --- end PR workspace ---

		// --- Diff pane ---
		DiffContext:      lipgloss.NewStyle(),
		DiffAdd:          lipgloss.NewStyle().Background(r.AddedTint),
		DiffDelete:       lipgloss.NewStyle().Background(r.RemovedTint),
		DiffCursor:       lipgloss.NewStyle().Background(r.Selection),
		DiffGutter:       fg(r.Muted),
		DiffGutterAdd:    fg(r.Added).Background(r.AddedGutterTint),
		DiffGutterDelete: fg(r.Removed).Background(r.RemovedGutterTint),
		DiffHunk:         fg(r.Info).Background(r.HunkTint),
		DiffHunkCursor:   bold(r.Info).Background(r.HunkCursorTint),
		DiffNoNewline:    fg(r.Muted).Italic(true),
		Syntax:           newSyntax(r),
		// --- end diff pane ---

		PRList: newPRListStyles(r),
	}
}
