package userinterface

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/theme/scheme"
)

// neverTick keeps status messages up for the whole test.
func neverTick(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }

// drive runs command and every command its results produce, feeding the
// messages back to the model the way the program would.
func drive(model *Model, command tea.Cmd) {
	if command == nil {
		return
	}
	switch message := command().(type) {
	case nil:
	case tea.BatchMsg:
		for _, inner := range message {
			drive(model, inner)
		}
	default:
		_, next := model.Update(message)
		drive(model, next)
	}
}

// themeModel is a model whose `:theme` loads come from loader, with the
// values asked for recorded.
func themeModel(root View, loader ThemeLoader) (*Model, *[]string) {
	var asked []string
	model := New(root, theme.Default(), Options{Tick: neverTick, LoadTheme: func(value string) (scheme.Scheme, error) {
		asked = append(asked, value)
		return loader(value)
	}})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	return model, &asked
}

func bundled(test *testing.T, slug string) scheme.Scheme {
	test.Helper()
	loaded, err := scheme.LoadBundled(slug)
	if err != nil {
		test.Fatal(err)
	}
	return loaded
}

func TestThemeCommandSwitchesTheme(test *testing.T) {
	root := newFake("v")
	tokyoNight := bundled(test, "tokyo-night")
	model, asked := themeModel(root, func(string) (scheme.Scheme, error) { return tokyoNight, nil })

	drive(model, parseCommand("theme  tokyo-night ", model.top()))

	if len(*asked) != 1 || (*asked)[0] != "tokyo-night" {
		test.Fatalf("loader asked for %q, want [tokyo-night]", *asked)
	}
	want := tokyoNight.Theme()
	if model.theme.Identifier != want.Identifier || !model.theme.Chosen {
		test.Error("the model did not switch to the loaded theme")
	}
	message, ok := root.last().(ThemeMessage)
	if !ok || message.Theme.Identifier != want.Identifier {
		test.Errorf("view's last message = %T, want a ThemeMessage with the new theme", root.last())
	}
	if !sameColour(model.commandLine.input.Styles().Focused.Text.GetForeground(), want.Roles.Text) {
		test.Error("the command line was not restyled")
	}
	if !sameColour(model.spin.Style.GetForeground(), want.Spinner.GetForeground()) {
		test.Error("the spinner was not restyled")
	}
	if row := ansi.Strip(model.bottomRow()); !strings.Contains(row, "theme: Tokyo Night") {
		test.Errorf("status row = %q, want it to name the new theme", row)
	}

	// A terminal background report arriving later never replaces it.
	model.Update(tea.BackgroundColorMsg{Color: color.White})
	if model.theme.Identifier != want.Identifier {
		test.Error("a late background reply replaced the theme chosen with :theme")
	}
}

func TestThemeCommandErrorKeepsTheme(test *testing.T) {
	root := newFake("v")
	model, _ := themeModel(root, scheme.LoadBundled)        // the real "unknown theme" error
	model.Update(tea.WindowSizeMsg{Width: 200, Height: 12}) // room for the whole error
	before := model.theme.Identifier
	seen := len(root.got)

	drive(model, parseCommand("theme nope", model.top()))

	if model.theme.Identifier != before {
		test.Error("a failed :theme changed the theme")
	}
	if len(root.got) != seen {
		test.Error("a failed :theme was broadcast to the views")
	}
	row := ansi.Strip(model.bottomRow())
	for _, want := range []string{`unknown theme "nope"`, "tokyo-night"} {
		if !strings.Contains(row, want) {
			test.Errorf("status row = %q, want it to contain %q", row, want)
		}
	}
}

func TestThemeCommandWithoutValue(test *testing.T) {
	model, asked := themeModel(newFake("v"), func(string) (scheme.Scheme, error) { return scheme.Scheme{}, nil })
	drive(model, parseCommand("theme", model.top()))
	if len(*asked) != 0 {
		test.Error("the loader ran without a value")
	}
	if row := ansi.Strip(model.bottomRow()); !strings.Contains(row, "usage: :theme <name or file>") {
		test.Errorf("status row = %q, want the usage", row)
	}
}

func TestThemeCommandStaleLoadIgnored(test *testing.T) {
	latte, tokyoNight := bundled(test, "catppuccin-latte"), bundled(test, "tokyo-night")
	model, _ := themeModel(newFake("v"), func(value string) (scheme.Scheme, error) {
		if value == "catppuccin-latte" {
			return latte, nil
		}
		return tokyoNight, nil
	})
	_, first := model.Update(themeRequestedMessage{value: "catppuccin-latte"})
	_, second := model.Update(themeRequestedMessage{value: "tokyo-night"})
	drive(model, second)
	drive(model, first) // the slower, superseded load finishes last
	if model.theme.Identifier != tokyoNight.Theme().Identifier {
		test.Error("a superseded :theme load replaced the later one")
	}
}

func TestLoadThemeFromWorkingDirectory(test *testing.T) {
	directory := test.TempDir()
	data, err := os.ReadFile("testdata/grey.yaml")
	if err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "mine.yaml"), data, 0o600); err != nil {
		test.Fatal(err)
	}
	test.Chdir(directory)

	loaded, err := LoadThemeFromWorkingDirectory("mine.yaml")
	if err != nil || loaded.Name != "Grey" {
		test.Errorf("relative path: %q, %v; want the file in the current directory", loaded.Name, err)
	}
	if loaded, err := LoadThemeFromWorkingDirectory("tokyo-night"); err != nil || loaded.Name != "Tokyo Night" {
		test.Errorf("slug: %q, %v", loaded.Name, err)
	}
	if _, err := LoadThemeFromWorkingDirectory("gone.yaml"); !errors.Is(err, os.ErrNotExist) {
		test.Error("a missing file is not an error")
	}
}
