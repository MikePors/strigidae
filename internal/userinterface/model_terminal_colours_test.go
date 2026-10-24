package userinterface

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// chosenTheme is a light theme marked as picked by the user, like a theme
// file's.
func chosenTheme() theme.Theme {
	chosen := theme.New(false)
	chosen.Chosen = true
	return chosen
}

func TestLateBackgroundReplyKeepsChosenTheme(test *testing.T) {
	root := newFake("v")
	model := New(root, chosenTheme(), Options{Tick: instantTick})
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	identifier := model.theme.Identifier
	seen := len(root.got)

	model.Update(tea.BackgroundColorMsg{Color: color.Black}) // a dark terminal answers late
	if model.theme.Identifier != identifier || !model.theme.Chosen {
		test.Error("a background reply replaced the chosen theme")
	}
	if len(root.got) != seen {
		test.Error("a background reply was broadcast as a theme change")
	}
}

func TestBackgroundReplyPicksBuiltInWhenNoThemeChosen(test *testing.T) {
	model := newModel(newFake("v"))
	model.Update(tea.BackgroundColorMsg{Color: color.White})
	if model.theme.IsDark || model.theme.Chosen {
		test.Error("a light background did not pick the built-in light theme")
	}
}

func TestTerminalColours(test *testing.T) {
	chosen := chosenTheme()
	model := New(newFake("v"), chosen, Options{Tick: instantTick})
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	view := model.View()
	if view.BackgroundColor == nil || !sameColour(view.BackgroundColor, chosen.Roles.Background) {
		test.Errorf("background = %v, want the background role", view.BackgroundColor)
	}
	if view.ForegroundColor == nil || !sameColour(view.ForegroundColor, chosen.Roles.Text) {
		test.Errorf("foreground = %v, want the text role", view.ForegroundColor)
	}

	builtIn := newModel(newFake("v")).View()
	if builtIn.BackgroundColor != nil || builtIn.ForegroundColor != nil {
		test.Error("a built-in theme sets the terminal colours")
	}
}
