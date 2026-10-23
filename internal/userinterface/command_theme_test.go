package userinterface

import (
	"image/color"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func sameColour(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar == br && ag == bg && ab == bb
}

// TestCommandLineFollowsTheme checks the command line's input styles come
// from the theme's roles (not bubbles' fixed ANSI defaults), for dark and
// light, and follow a theme change.
func TestCommandLineFollowsTheme(t *testing.T) {
	for name, th := range map[string]theme.Theme{"dark": theme.New(true), "light": theme.New(false)} {
		c := newCommandLine(th)
		st := c.input.Styles()
		for state, s := range map[string]textinput.StyleState{"focused": st.Focused, "blurred": st.Blurred} {
			if !sameColour(s.Text.GetForeground(), th.Roles.Text) {
				t.Errorf("%s %s: text is not the text role", name, state)
			}
			if !sameColour(s.Prompt.GetForeground(), th.Roles.Accent) {
				t.Errorf("%s %s: prompt is not the accent role", name, state)
			}
			if !sameColour(s.Text.GetBackground(), th.Roles.Surface) {
				t.Errorf("%s %s: text is not drawn on the surface role", name, state)
			}
		}
		if !sameColour(st.Cursor.Color, th.Roles.Text) {
			t.Errorf("%s: cursor is not the text role", name)
		}
	}

	m := newModel(newFake("v"))
	m.Update(tea.BackgroundColorMsg{Color: color.White})
	if !sameColour(m.cmdline.input.Styles().Focused.Text.GetForeground(), theme.New(false).Roles.Text) {
		t.Error("a light background did not restyle the command line")
	}
}
