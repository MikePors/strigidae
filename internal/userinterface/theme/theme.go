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
// style is generated from the syntax roles, unless the theme names a bundled
// Chroma style (see Syntax).
package theme

import (
	"image/color"
	"slices"
	"strings"

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
	HelpTitle       lipgloss.Style
	HelpSection     lipgloss.Style
	HelpKey         lipgloss.Style
	HelpDescription lipgloss.Style

	// General content.
	Title    lipgloss.Style // screen headings
	Normal   lipgloss.Style // plain text
	Muted    lipgloss.Style // de-emphasised text
	Selected lipgloss.Style // the highlighted row in a list
	// Fade is the steps of something fading out, from just off its normal
	// colour (index 0) to almost the background (the last); see fade.go.
	Fade [FadeSteps]lipgloss.Style

	// Chosen is set for a theme the user picked (a bundled theme, a theme
	// file or the terminal's palette), as opposed to a built-in one. A
	// terminal's reported background never replaces a chosen theme, and one
	// that isn't FromTerminal also sets the terminal's own background and
	// foreground.
	Chosen bool
	// FromTerminal is set for a theme built from the terminal's own palette
	// (`theme: terminal`), which already matches the terminal's colours.
	FromTerminal bool
	// IsDark reports whether the palette is the dark variant.
	IsDark bool
	// Identifier fingerprints the final roles, the variant, the named syntax
	// style and, with a named style, which syn-* roles the theme overrides, so
	// any change to a role (including an override) or to the syntax style
	// changes it. Renderer caches include it.
	Identifier string
	// Roles are the colours the styles were built from. Renderers with their
	// own style systems (Markdown) read them; views use the named styles.
	Roles Roles

	// --- PR workspace ---
	PaneBorder       lipgloss.Style // frame of an unfocused pane (the border role)
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
	SearchMarker     lipgloss.Style // the mark in the left column of rows that match the search
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
	Syntax           Syntax         // token styles, for the diff pane and Markdown code blocks
	ThreadMarker     lipgloss.Style // gutter marker and header of an open review thread
	ThreadBar        lipgloss.Style // left bar of a thread's comment lines
	// --- end diff pane ---

	PullRequestList PullRequestListStyles // pull request tables
	Dashboard       DashboardStyles       // the dashboard view
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
func FromPalette(palette Palette) Theme { return FromPaletteWithOverrides(palette, Roles{}) }

// FromPaletteWithOverrides builds a theme from a palette, replacing every
// role that overrides sets (non-nil fields) with its exact colour.
func FromPaletteWithOverrides(palette Palette, overrides Roles) Theme {
	return newTheme(palette, deriveRoles(palette, overrides), "", overrides.set())
}

// FromPaletteWithSyntax is FromPaletteWithOverrides with syntax colours from
// the named bundled Chroma style instead of the syntax roles; a syn-<class>
// override still recolours its class. An empty name generates the style from
// the palette. An unknown name is an error that lists the bundled styles.
func FromPaletteWithSyntax(palette Palette, overrides Roles, syntax string) (Theme, error) {
	if syntax != "" {
		if err := CheckSyntaxStyle(syntax); err != nil {
			return Theme{}, err
		}
	}
	return newTheme(palette, deriveRoles(palette, overrides), strings.ToLower(syntax), overrides.set()), nil
}

func newTheme(palette Palette, roles Roles, syntax string, overridden map[string]bool) Theme {
	identifier := roles.fingerprint(palette.Variant, syntaxIdentity(syntax, overridden))
	bar := lipgloss.NewStyle().Background(roles.Surface).Foreground(roles.Text)
	foreground := func(colour color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(colour) }
	bold := func(colour color.Color) lipgloss.Style { return foreground(colour).Bold(true) }

	return Theme{
		StatusBar:     bar,
		StatusContext: bar.Bold(true).Foreground(roles.OnAccent).Background(roles.Accent),
		StatusPending: bar.Bold(true).Foreground(roles.Warning),
		StatusInfo:    bar.Foreground(roles.Success),
		StatusError:   bar.Bold(true).Foreground(roles.Error),
		StatusHint:    bar.Foreground(roles.Muted),
		StatusCommand: bar.Bold(true),
		CommandPrompt: bar.Bold(true).Foreground(roles.Accent),
		CommandText:   bar.Foreground(roles.Text),
		CommandHint:   bar.Foreground(roles.Muted),
		Spinner:       bar.Foreground(roles.Accent),

		HeadingMarker: foreground(roles.Muted),

		HelpTitle:       bold(roles.Accent),
		HelpSection:     lipgloss.NewStyle().Bold(true).Underline(true),
		HelpKey:         foreground(roles.Warning),
		HelpDescription: lipgloss.NewStyle(),

		Title:      bold(roles.Accent),
		Normal:     lipgloss.NewStyle(),
		Muted:      foreground(roles.Muted),
		Selected:   bold(roles.AccentStrong),
		Fade:       fadeStyles(roles),
		IsDark:     palette.Variant == Dark,
		Identifier: identifier,
		Roles:      roles,

		// --- PR workspace ---
		PaneBorder:       foreground(roles.Border),
		PaneBorderFocus:  foreground(roles.Accent),
		PaneTitle:        foreground(roles.Muted),
		PaneTitleFocus:   bold(roles.OnAccent).Background(roles.Accent),
		FileAdded:        foreground(roles.Added),
		FileDeleted:      foreground(roles.Removed),
		FileModified:     foreground(roles.Warning),
		StateOpen:        bold(roles.Success),
		StateDraft:       bold(roles.Muted),
		StateMerged:      bold(roles.Accent),
		StateClosed:      bold(roles.Error),
		Author:           lipgloss.NewStyle().Bold(true),
		Pending:          bold(roles.Warning),
		ErrorText:        bold(roles.Error),
		SectionHeading:   bold(roles.Accent),
		SearchMarker:     bold(roles.Accent),
		OverlayBorder:    foreground(roles.Accent),
		OverlayBox:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(roles.Accent),
		StrongText:       lipgloss.NewStyle().Bold(true),
		OverlayItem:      lipgloss.NewStyle(),
		OverlayItemFocus: bold(roles.AccentStrong).Reverse(true),
		// --- end PR workspace ---

		// --- Diff pane ---
		DiffContext:      lipgloss.NewStyle(),
		DiffAdd:          lipgloss.NewStyle().Background(roles.AddedTint),
		DiffDelete:       lipgloss.NewStyle().Background(roles.RemovedTint),
		DiffCursor:       lipgloss.NewStyle().Background(roles.Selection),
		DiffGutter:       foreground(roles.Muted),
		DiffGutterAdd:    foreground(roles.Added).Background(roles.AddedGutterTint),
		DiffGutterDelete: foreground(roles.Removed).Background(roles.RemovedGutterTint),
		DiffHunk:         foreground(roles.Info).Background(roles.HunkTint),
		DiffHunkCursor:   bold(roles.Info).Background(roles.HunkCursorTint),
		DiffNoNewline:    foreground(roles.Muted).Italic(true),
		Syntax:           newSyntax(syntax, generatedPrefix+identifier, roles, overridden),
		ThreadMarker:     bold(roles.Warning),
		ThreadBar:        foreground(roles.Info),
		// --- end diff pane ---

		PullRequestList: newPullRequestListStyles(roles),
		Dashboard:       newDashboardStyles(roles),
	}
}

// syntaxIdentity is what the syntax style adds to Theme.Identifier: its name
// and, for a named style, which syn-* roles the theme overrides. Overriding a
// role with the colour it already had changes nothing in the roles, but it
// recolours that whole class of the named style.
func syntaxIdentity(syntax string, overridden map[string]bool) string {
	if syntax == "" {
		return ""
	}
	names := []string{syntax}
	for name := range overridden {
		if strings.HasPrefix(name, "syn-") {
			names = append(names, name)
		}
	}
	slices.Sort(names[1:])
	return strings.Join(names, "|")
}
