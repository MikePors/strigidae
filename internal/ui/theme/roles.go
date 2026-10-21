package theme

import (
	"image/color"
	"strings"
)

// Roles are what strig means by a colour. Each has a default derivation from
// palette slots (deriveRoles, the only code that reads slots), and a theme may
// override any role with an exact colour. Styles are built from roles only.
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

// Tint ratios: how much of the role shows through a row background.
const (
	tintRow    = 0.12
	tintGutter = 0.24
	tintHunk   = 0.10
	tintCursor = 0.22
)

// deriveRoles computes every role from the palette, then applies overrides
// (non-nil fields of ov), then runs the contrast guard.
func deriveRoles(p Palette, ov Roles) Roles {
	r := defaultRoles(p)
	r.override(ov)
	r.guard()
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
	r.deriveTints()
	return r
}

func (r *Roles) deriveTints() {
	r.AddedTint = blend(r.Background, r.Added, tintRow)
	r.AddedGutterTint = blend(r.Background, r.Added, tintGutter)
	r.RemovedTint = blend(r.Background, r.Removed, tintRow)
	r.RemovedGutterTint = blend(r.Background, r.Removed, tintGutter)
	r.HunkTint = blend(r.Background, r.Info, tintHunk)
	r.HunkCursorTint = blend(r.Background, r.Info, tintCursor)
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

// override replaces every role ov sets. Tints that ov does not set are
// recomputed from the final roles, so overriding Added also recolours the
// added-line tint.
func (r *Roles) override(ov Roles) {
	ovf := ov.fields()
	for i, f := range r.fields() {
		if *ovf[i].c != nil {
			*f.c = *ovf[i].c
		}
	}
	derived := *r
	derived.deriveTints()
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

// guard nudges foreground roles that are unreadable on the colours they are
// drawn over. A role that already passes is untouched, so a sound palette
// comes out exactly as derived.
func (r *Roles) guard() {
	// Text sits on the page and on the status bar.
	for _, bg := range []color.Color{r.Background, r.Surface} {
		r.Text = nudge(r.Text, bg, minTextContrast)
		r.Muted = nudge(r.Muted, bg, minAccentContrast)
	}
	for _, c := range []*color.Color{
		&r.Accent, &r.AccentStrong, &r.Info, &r.Link, &r.Warning, &r.Error, &r.Success,
		&r.Added, &r.Removed,
		&r.SynKeyword, &r.SynString, &r.SynComment, &r.SynNumber, &r.SynFunction,
		&r.SynType, &r.SynOperator, &r.SynVariable, &r.SynEscape,
	} {
		*c = nudge(*c, r.Background, minAccentContrast)
	}
	r.OnAccent = nudge(r.OnAccent, r.Accent, minAccentContrast)
}
