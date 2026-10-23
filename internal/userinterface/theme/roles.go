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
	SynKeyword  color.Color
	SynString   color.Color
	SynComment  color.Color
	SynNumber   color.Color // numbers and constants
	SynFunction color.Color
	SynType     color.Color
	SynOperator color.Color
	SynVariable color.Color // variables and tags
	SynEscape   color.Color // string escapes and regular expressions
}

// tintParams say how a tint is made from a role: how far the background's
// lightness moves toward the role's, and how much of the role's chroma it
// keeps (see tint).
type tintParams struct{ lightMix, chroma float64 }

// tintSet holds the parameters for each tint. They differ per variant because
// the same blend reads very differently on a dark and a light background.
type tintSet struct{ row, gutter, hunk, hunkCursor tintParams }

func tintsFor(v Variant) tintSet {
	// Fitted so the built-in palettes reproduce the earlier hand-picked row
	// colours closely: deep green and red on dark, pastel mint and pink on
	// light, with the same hunk-header shades.
	if v == Light {
		return tintSet{
			row:        tintParams{0.07, 0.20},
			gutter:     tintParams{0.085, 0.52},
			hunk:       tintParams{0.09, 0.27},
			hunkCursor: tintParams{0.20, 0.38},
		}
	}
	return tintSet{
		row:        tintParams{0.03, 0.24},
		gutter:     tintParams{0.13, 0.38},
		hunk:       tintParams{0.10, 0.31},
		hunkCursor: tintParams{0.29, 0.49},
	}
}

// deriveRoles computes every role from the palette and applies the overrides
// (non-nil fields of ov). The contrast guard then adjusts only the roles that
// were derived: an override is used exactly as written, and a warning is
// logged if it is hard to read.
func deriveRoles(p Palette, ov Roles) Roles {
	r := defaultRoles(p)
	r.override(ov, tintsFor(p.Variant))
	r.guard(ov.set())
	return r
}

func defaultRoles(p Palette) Roles {
	s := p.Slot
	r := Roles{
		Background: s(Base00),
		Surface:    s(Base01),
		Text:       s(Base05),
		Muted:      s(Base03),
		Selection:  s(Base02),

		Accent:       s(Base0E),
		AccentStrong: blend(s(Base0E), s(Base07), 0.4),
		OnAccent:     s(Base00),
		Info:         s(Base0D),
		Link:         s(Base0C),
		Warning:      s(Base09),
		Error:        s(Base08),
		Success:      s(Base0B),
		Added:        s(Base0B),
		Removed:      s(Base08),

		SynKeyword:  s(Base0E),
		SynString:   s(Base0B),
		SynComment:  s(Base03),
		SynNumber:   s(Base09),
		SynFunction: s(Base0D),
		SynType:     s(Base0A),
		SynOperator: s(Base0C),
		SynVariable: s(Base08),
		SynEscape:   s(Base0C),
	}
	r.deriveTints(tintsFor(p.Variant))
	return r
}

func (r *Roles) deriveTints(t tintSet) {
	mk := func(role color.Color, p tintParams) color.Color {
		return tint(r.Background, role, p.lightMix, p.chroma)
	}
	r.AddedTint = mk(r.Added, t.row)
	r.AddedGutterTint = mk(r.Added, t.gutter)
	r.RemovedTint = mk(r.Removed, t.row)
	r.RemovedGutterTint = mk(r.Removed, t.gutter)
	r.HunkTint = mk(r.Info, t.hunk)
	r.HunkCursorTint = mk(r.Info, t.hunkCursor)
}

// fields lists every role by name, for overriding, guarding and tests.
func (r *Roles) fields() []roleField {
	return []roleField{
		{"background", &r.Background},
		{"surface", &r.Surface},
		{"text", &r.Text},
		{"muted", &r.Muted},
		{"selection", &r.Selection},
		{"accent", &r.Accent},
		{"accent-strong", &r.AccentStrong},
		{"on-accent", &r.OnAccent},
		{"info", &r.Info},
		{"link", &r.Link},
		{"warning", &r.Warning},
		{"error", &r.Error},
		{"success", &r.Success},
		{"added", &r.Added},
		{"removed", &r.Removed},
		{"added-tint", &r.AddedTint},
		{"added-gutter-tint", &r.AddedGutterTint},
		{"removed-tint", &r.RemovedTint},
		{"removed-gutter-tint", &r.RemovedGutterTint},
		{"hunk-tint", &r.HunkTint},
		{"hunk-cursor-tint", &r.HunkCursorTint},
		{"syn-keyword", &r.SynKeyword},
		{"syn-string", &r.SynString},
		{"syn-comment", &r.SynComment},
		{"syn-number", &r.SynNumber},
		{"syn-function", &r.SynFunction},
		{"syn-type", &r.SynType},
		{"syn-operator", &r.SynOperator},
		{"syn-variable", &r.SynVariable},
		{"syn-escape", &r.SynEscape},
	}
}

type roleField struct {
	name string
	c    *color.Color
}

// set returns the names of the roles r overrides (its non-nil fields).
func (r Roles) set() map[string]bool {
	out := map[string]bool{}
	for _, f := range r.fields() {
		if *f.c != nil {
			out[f.name] = true
		}
	}
	return out
}

// override replaces every role ov sets. Tints that ov does not set are
// recomputed from the final roles, so overriding Added also recolours the
// added-line tint.
func (r *Roles) override(ov Roles, t tintSet) {
	ovf := ov.fields()
	for i, f := range r.fields() {
		if *ovf[i].c != nil {
			*f.c = *ovf[i].c
		}
	}
	derived := *r
	derived.deriveTints(t)
	df := derived.fields()
	for i, f := range r.fields() {
		if strings.HasSuffix(f.name, "-tint") && *ovf[i].c == nil {
			*f.c = *df[i].c
		}
	}
}

// Minimum WCAG contrast ratios: body text, then everything else that is drawn
// as a foreground.
const (
	minTextContrast   = 4.5
	minAccentContrast = 3.0
)

// guard nudges derived foreground roles that are unreadable on the colours
// they are drawn over. A role that already passes is untouched, so a sound
// palette comes out exactly as derived. A role in overridden is never
// changed: it is the theme author's exact choice, so a failing one is only
// logged.
func (r *Roles) guard(overridden map[string]bool) {
	check := func(name string, fg *color.Color, floor float64, bgs ...color.Color) {
		worst := math.Inf(1)
		for _, bg := range bgs {
			worst = math.Min(worst, contrast(*fg, bg))
		}
		if worst >= floor {
			return
		}
		if overridden[name] {
			slog.Warn("theme role is hard to read", "role", name, "contrast", math.Round(worst*100)/100, "want", floor)
			return
		}
		for _, bg := range bgs {
			*fg = nudge(*fg, bg, floor)
		}
	}
	// Text sits on the page and on the status bar.
	check("text", &r.Text, minTextContrast, r.Background, r.Surface)
	check("muted", &r.Muted, minAccentContrast, r.Background, r.Surface)
	for _, f := range []roleField{
		{"accent", &r.Accent},
		{"accent-strong", &r.AccentStrong},
		{"info", &r.Info},
		{"link", &r.Link},
		{"warning", &r.Warning},
		{"error", &r.Error},
		{"success", &r.Success},
		{"added", &r.Added},
		{"removed", &r.Removed},
		{"syn-keyword", &r.SynKeyword},
		{"syn-string", &r.SynString},
		{"syn-comment", &r.SynComment},
		{"syn-number", &r.SynNumber},
		{"syn-function", &r.SynFunction},
		{"syn-type", &r.SynType},
		{"syn-operator", &r.SynOperator},
		{"syn-variable", &r.SynVariable},
		{"syn-escape", &r.SynEscape},
	} {
		check(f.name, f.c, minAccentContrast, r.Background)
	}
	check("on-accent", &r.OnAccent, minAccentContrast, r.Accent)
}

// fingerprint identifies the final roles and the variant. Every style and the
// generated Chroma style are functions of them, so it is the identity
// rendered-output caches key on. extra carries anything else that affects
// rendering, such as a named syntax style.
func (r *Roles) fingerprint(v Variant, extra string) string {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d|%s", v, extra)
	for _, f := range r.fields() {
		_, _ = h.Write([]byte(HexString(*f.c)))
	}
	return fmt.Sprintf("%016x", h.Sum64())
}
