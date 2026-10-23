package userinterface

import (
	"image/color"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func sameColour(first, second color.Color) bool {
	firstRed, firstGreen, firstBlue, _ := first.RGBA()
	secondRed, secondGreen, secondBlue, _ := second.RGBA()
	return firstRed == secondRed && firstGreen == secondGreen && firstBlue == secondBlue
}

// TestCommandLineFollowsTheme checks the command line's input styles come
// from the theme's roles (not bubbles' fixed ANSI defaults), for dark and
// light, and follow a theme change.
func TestCommandLineFollowsTheme(test *testing.T) {
	for name, activeTheme := range map[string]theme.Theme{"dark": theme.New(true), "light": theme.New(false)} {
		commandLine := newCommandLine(activeTheme)
		styles := commandLine.input.Styles()
		for state, style := range map[string]textinput.StyleState{"focused": styles.Focused, "blurred": styles.Blurred} {
			if !sameColour(style.Text.GetForeground(), activeTheme.Roles.Text) {
				test.Errorf("%s %s: text is not the text role", name, state)
			}
			if !sameColour(style.Prompt.GetForeground(), activeTheme.Roles.Accent) {
				test.Errorf("%s %s: prompt is not the accent role", name, state)
			}
			if !sameColour(style.Text.GetBackground(), activeTheme.Roles.Surface) {
				test.Errorf("%s %s: text is not drawn on the surface role", name, state)
			}
		}
		if !sameColour(styles.Cursor.Color, activeTheme.Roles.Text) {
			test.Errorf("%s: cursor is not the text role", name)
		}
	}

	model := newModel(newFake("v"))
	model.Update(tea.BackgroundColorMsg{Color: color.White})
	if !sameColour(model.cmdline.input.Styles().Focused.Text.GetForeground(), theme.New(false).Roles.Text) {
		test.Error("a light background did not restyle the command line")
	}
}
