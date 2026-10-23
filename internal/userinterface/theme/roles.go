package theme

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"log/slog"
	"math"
	"strings"
)

// Roles are what strig means by a colour. Each has a default derivation from
// palette slots (deriveRoles, the only code that reads slots), and a theme may
// override any role with an exact colour (used as written, never adjusted).
// Styles are built from roles only.
type Roles struct {
	// Surfaces and text.
	Background color.Color // base of every tint; the page the palette was designed for
	Surface    color.Color // status bar and code spans
	Text       color.Color // body text
	Muted      color.Color // de-emphasised text, borders, line numbers
	Selection  color.Color // the cursor row's background

	// Accents.
	Accent       color.Color // focus, headings, merged state
	AccentStrong color.Color // selected item text, brighter than Accent
	OnAccent     color.Color // text drawn on an Accent background
	Info         color.Color // hunk headers
	Link         color.Color // hyperlinks
	Warning      color.Color // pending, numbers, key hints, modified files
	Error        color.Color // failures
	Success      color.Color // success messages, open state, approvals
	Added        color.Color // added lines and files
	Removed      color.Color // deleted lines and files

	// Tints: a role blended into Background at a fixed ratio, for row
	// backgrounds.
	AddedTint         color.Color
	AddedGutterTint   color.Color
	RemovedTint       color.Color
	RemovedGutterTint color.Color
	HunkTint          color.Color
	HunkCursorTint    color.Color

	// Syntax token colours.
	SyntaxKeyword  color.Color
	SyntaxString   color.Color
	SyntaxComment  color.Color
	SyntaxNumber   color.Color // numbers and constants
	SyntaxFunction color.Color
	SyntaxType     color.Color
	SyntaxOperator color.Color
	SyntaxVariable color.Color // variables and tags
	SyntaxEscape   color.Color // string escapes and regular expressions
}

// tintParameters say how a tint is made from a role: how far the background's
// lightness moves toward the role's, and how much of the role's chroma it
// keeps (see tint).
type tintParameters struct{ lightMix, chroma float64 }

// tintSet holds the parameters for each tint. They differ per variant because
// the same blend reads very differently on a dark and a light background.
type tintSet struct{ row, gutter, hunk, hunkCursor tintParameters }

func tintsFor(variant Variant) tintSet {
	// Fitted so the built-in palettes reproduce the earlier hand-picked row
	// colours closely: deep green and red on dark, pastel mint and pink on
	// light, with the same hunk-header shades.
	if variant == Light {
		return tintSet{
			row:        tintParameters{0.07, 0.20},
			gutter:     tintParameters{0.085, 0.52},
			hunk:       tintParameters{0.09, 0.27},
			hunkCursor: tintParameters{0.20, 0.38},
		}
	}
	return tintSet{
		row:        tintParameters{0.03, 0.24},
		gutter:     tintParameters{0.13, 0.38},
		hunk:       tintParameters{0.10, 0.31},
		hunkCursor: tintParameters{0.29, 0.49},
	}
}

// deriveRoles computes every role from the palette and applies the overrides
// (non-nil fields of ov). The contrast guard then adjusts only the roles that
// were derived: an override is used exactly as written, and a warning is
// logged if it is hard to read.
func deriveRoles(palette Palette, overrides Roles) Roles {
	roles := defaultRoles(palette)
	roles.override(overrides, tintsFor(palette.Variant))
	roles.guard(overrides.set())
	return roles
}

func defaultRoles(palette Palette) Roles {
	colorOf := palette.Slot
	roles := Roles{
		Background: colorOf(Base00),
		Surface:    colorOf(Base01),
		Text:       colorOf(Base05),
		Muted:      colorOf(Base03),
		Selection:  colorOf(Base02),

		Accent:       colorOf(Base0E),
		AccentStrong: blend(colorOf(Base0E), colorOf(Base07), 0.4),
		OnAccent:     colorOf(Base00),
		Info:         colorOf(Base0D),
		Link:         colorOf(Base0C),
		Warning:      colorOf(Base09),
		Error:        colorOf(Base08),
		Success:      colorOf(Base0B),
		Added:        colorOf(Base0B),
		Removed:      colorOf(Base08),

		SyntaxKeyword:  colorOf(Base0E),
		SyntaxString:   colorOf(Base0B),
		SyntaxComment:  colorOf(Base03),
		SyntaxNumber:   colorOf(Base09),
		SyntaxFunction: colorOf(Base0D),
		SyntaxType:     colorOf(Base0A),
		SyntaxOperator: colorOf(Base0C),
		SyntaxVariable: colorOf(Base08),
		SyntaxEscape:   colorOf(Base0C),
	}
	roles.deriveTints(tintsFor(palette.Variant))
	return roles
}

func (roles *Roles) deriveTints(tints tintSet) {
	makeColor := func(role color.Color, parameters tintParameters) color.Color {
		return tint(roles.Background, role, parameters.lightMix, parameters.chroma)
	}
	roles.AddedTint = makeColor(roles.Added, tints.row)
	roles.AddedGutterTint = makeColor(roles.Added, tints.gutter)
	roles.RemovedTint = makeColor(roles.Removed, tints.row)
	roles.RemovedGutterTint = makeColor(roles.Removed, tints.gutter)
	roles.HunkTint = makeColor(roles.Info, tints.hunk)
	roles.HunkCursorTint = makeColor(roles.Info, tints.hunkCursor)
}

// fields lists every role by name, for overriding, guarding and tests.
func (roles *Roles) fields() []roleField {
	return []roleField{
		{"background", &roles.Background},
		{"surface", &roles.Surface},
		{"text", &roles.Text},
		{"muted", &roles.Muted},
		{"selection", &roles.Selection},
		{"accent", &roles.Accent},
		{"accent-strong", &roles.AccentStrong},
		{"on-accent", &roles.OnAccent},
		{"info", &roles.Info},
		{"link", &roles.Link},
		{"warning", &roles.Warning},
		{"error", &roles.Error},
		{"success", &roles.Success},
		{"added", &roles.Added},
		{"removed", &roles.Removed},
		{"added-tint", &roles.AddedTint},
		{"added-gutter-tint", &roles.AddedGutterTint},
		{"removed-tint", &roles.RemovedTint},
		{"removed-gutter-tint", &roles.RemovedGutterTint},
		{"hunk-tint", &roles.HunkTint},
		{"hunk-cursor-tint", &roles.HunkCursorTint},
		{"syn-keyword", &roles.SyntaxKeyword},
		{"syn-string", &roles.SyntaxString},
		{"syn-comment", &roles.SyntaxComment},
		{"syn-number", &roles.SyntaxNumber},
		{"syn-function", &roles.SyntaxFunction},
		{"syn-type", &roles.SyntaxType},
		{"syn-operator", &roles.SyntaxOperator},
		{"syn-variable", &roles.SyntaxVariable},
		{"syn-escape", &roles.SyntaxEscape},
	}
}

type roleField struct {
	name  string
	color *color.Color
}

// set returns the names of the roles r overrides (its non-nil fields).
func (roles Roles) set() map[string]bool {
	out := map[string]bool{}
	for _, field := range roles.fields() {
		if *field.color != nil {
			out[field.name] = true
		}
	}
	return out
}

// override replaces every role ov sets. Tints that ov does not set are
// recomputed from the final roles, so overriding Added also recolours the
// added-line tint.
func (roles *Roles) override(overrides Roles, tints tintSet) {
	overridden := overrides.fields()
	for i, field := range roles.fields() {
		if *overridden[i].color != nil {
			*field.color = *overridden[i].color
		}
	}
	derived := *roles
	derived.deriveTints(tints)
	defaults := derived.fields()
	for i, field := range roles.fields() {
		if strings.HasSuffix(field.name, "-tint") && *overridden[i].color == nil {
			*field.color = *defaults[i].color
		}
	}
}

// Minimum WCAG contrast ratios: body text, then everything else that is drawn
// as a foreground.
const (
	minimumTextContrast   = 4.5
	minimumAccentContrast = 3.0
)

// guard nudges derived foreground roles that are unreadable on the colours
// they are drawn over. A role that already passes is untouched, so a sound
// palette comes out exactly as derived. A role in overridden is never
// changed: it is the theme author's exact choice, so a failing one is only
// logged.
func (roles *Roles) guard(overridden map[string]bool) {
	check := func(name string, foreground *color.Color, floor float64, colors ...color.Color) {
		worst := math.Inf(1)
		for _, background := range colors {
			worst = math.Min(worst, contrast(*foreground, background))
		}
		if worst >= floor {
			return
		}
		if overridden[name] {
			slog.Warn("theme role is hard to read", "role", name, "contrast", math.Round(worst*100)/100, "want", floor)
			return
		}
		for _, background := range colors {
			*foreground = nudge(*foreground, background, floor)
		}
	}
	// Text sits on the page and on the status bar.
	check("text", &roles.Text, minimumTextContrast, roles.Background, roles.Surface)
	check("muted", &roles.Muted, minimumAccentContrast, roles.Background, roles.Surface)
	for _, field := range []roleField{
		{"accent", &roles.Accent},
		{"accent-strong", &roles.AccentStrong},
		{"info", &roles.Info},
		{"link", &roles.Link},
		{"warning", &roles.Warning},
		{"error", &roles.Error},
		{"success", &roles.Success},
		{"added", &roles.Added},
		{"removed", &roles.Removed},
		{"syn-keyword", &roles.SyntaxKeyword},
		{"syn-string", &roles.SyntaxString},
		{"syn-comment", &roles.SyntaxComment},
		{"syn-number", &roles.SyntaxNumber},
		{"syn-function", &roles.SyntaxFunction},
		{"syn-type", &roles.SyntaxType},
		{"syn-operator", &roles.SyntaxOperator},
		{"syn-variable", &roles.SyntaxVariable},
		{"syn-escape", &roles.SyntaxEscape},
	} {
		check(field.name, field.color, minimumAccentContrast, roles.Background)
	}
	check("on-accent", &roles.OnAccent, minimumAccentContrast, roles.Accent)
}

// fingerprint identifies the final roles and the variant. Every style and the
// generated Chroma style are functions of them, so it is the identity
// rendered-output caches key on. extra carries anything else that affects
// rendering, such as a named syntax style.
func (roles *Roles) fingerprint(variant Variant, extra string) string {
	hasher := fnv.New64a()
	_, _ = fmt.Fprintf(hasher, "%d|%s", variant, extra)
	for _, field := range roles.fields() {
		_, _ = hasher.Write([]byte(HexString(*field.color)))
	}
	return fmt.Sprintf("%016x", hasher.Sum64())
}
