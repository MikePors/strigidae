// Package scheme reads colour themes from Base16 and Base24 scheme files in
// YAML (the tinted-theming format) and from the themes bundled with strig. A
// scheme file may also carry two strig extensions: roles, exact colours for
// individual roles, and syntax, a named syntax style.
//
// It turns a file into a theme.Palette plus role overrides; building the
// theme from them stays in package theme, so colours reach styles only
// through the palette and role layers.
package scheme

import (
	"fmt"
	"log/slog"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// Scheme is a parsed theme file.
type Scheme struct {
	Name    string
	Palette theme.Palette
	// Overrides holds the roles the file sets exactly (the nil fields are
	// derived from the palette).
	Overrides theme.Roles
	// Syntax names the bundled Chroma style the file asks for, checked when
	// the file is parsed; empty means the style is generated from the palette.
	Syntax string
	// Warnings are problems that don't stop the theme loading, for the caller
	// to log.
	Warnings []string
}

// Theme builds the theme. It is marked Chosen: it sets the terminal's own
// background and foreground, and a terminal background report never replaces it.
func (scheme Scheme) Theme() theme.Theme {
	built, err := theme.FromPaletteWithSyntax(scheme.Palette, scheme.Overrides, scheme.Syntax)
	if err != nil { // Parse already checked Syntax; only a hand-built Scheme gets here
		slog.Warn("theme syntax style ignored", "theme", scheme.Name, "error", err)
		built = theme.FromPaletteWithOverrides(scheme.Palette, scheme.Overrides)
	}
	built.Chosen = true
	return built
}

// Error is a problem in a theme file, located by line.
type Error struct {
	Source  string // the file path, or the bundled theme's file name
	Line    int    // 1-based; 0 if unknown
	Message string
}

func (failure *Error) Error() string {
	if failure.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", failure.Source, failure.Line, failure.Message)
	}
	return failure.Source + ": " + failure.Message
}
