package userinterface

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

func TestTerminalPaletteThemeLeavesTerminalColours(test *testing.T) {
	fromTerminal := chosenTheme()
	fromTerminal.FromTerminal = true
	model := New(newFake("v"), fromTerminal, Options{Tick: instantTick})
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	if view := model.View(); view.BackgroundColor != nil || view.ForegroundColor != nil {
		test.Error("a theme built from the terminal's palette sets the terminal colours")
	}
	identifier := model.theme.Identifier
	model.Update(tea.BackgroundColorMsg{Color: color.Black})
	if model.theme.Identifier != identifier {
		test.Error("a background reply replaced the theme built from the terminal's palette")
	}
}

func TestStartupNoticeShown(test *testing.T) {
	model := New(newFake("v"), theme.Default(), Options{Tick: instantTick, Notice: "theme terminal: no answer from the terminal"})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	batch, ok := model.Init()().(tea.BatchMsg)
	if !ok {
		test.Fatal("Init did not return a batch")
	}
	for _, command := range batch {
		run(model, command)
	}
	if row := ansi.Strip(model.bottomRow()); !strings.Contains(row, "theme terminal: no answer from the terminal") {
		test.Errorf("status row = %q, want the notice", row)
	}
}
