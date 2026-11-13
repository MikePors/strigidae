package userinterface

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/golden"
)

func TestCommandHelp(test *testing.T) {
	for _, testCase := range []struct {
		name string
		want string
	}{
		{"set", ":set [option ... | all]"},
		{"quit", "close the current view"},
		{":quit", "close the current view"},
		{"edit-comment", "Keys: e"},
		{"config", "wrapscan"},
		{":config", "describe this plugin"},
		{"ask", "run the configured plugin on lines"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			model := sizedModel(100, 24)
			model.commandHelp = []CommandHelp{{Name: "edit-comment", Description: "edit the selected comment", Keys: []string{"e"}, View: "Pull request workspace"}}
			model.pluginCommands = []PluginCommand{{Name: "config", Description: "describe this plugin"}, {Name: "ask", Target: config.TargetLines}}
			enterLine(model, "help "+testCase.name)
			if !model.showHelp || !strings.Contains(screen(model), testCase.want) {
				test.Fatalf("wanted %q in help:\n%s", testCase.want, screen(model))
			}
			if testCase.name == "edit-comment" {
				golden.Assert(test, "help_command", screen(model), false)
			}
		})
	}
}

func TestCommandHelpCompletionAcrossViews(test *testing.T) {
	model := sizedModel(100, 24)
	model.commandHelp = []CommandHelp{{Name: "merge"}, {Name: "copy-url"}, {Name: "copy-url"}}
	model.pluginCommands = []PluginCommand{{Name: "config"}, {Name: "ask"}}
	_, candidates, _ := model.completionCandidates("help ")
	for _, name := range []string{"merge", "copy-url", "config", ":config", "ask", "quit"} {
		if !slices.Contains(candidates, name) {
			test.Errorf("missing %s from %v", name, candidates)
		}
	}
	count := 0
	for _, candidate := range candidates {
		if candidate == "copy-url" {
			count++
		}
	}
	if count != 1 {
		test.Errorf("copy-url appears %d times, want once: %v", count, candidates)
	}
	if slices.Contains(candidates, ":copy-url") {
		test.Errorf("duplicate view created a false topic clash: %v", candidates)
	}
	typeKeys(model, "help :conf", "tab")
	if got := model.commandLine.input.Value(); got != "help :config" {
		test.Errorf("completion = %q", got)
	}
}

func TestCommandHelpWrapsAndCleansPluginText(test *testing.T) {
	model := sizedModel(100, 30)
	model.pluginCommands = []PluginCommand{{Name: "ask", Description: "Explain \x1b[31mthis long plugin description with several words and no terminal escapes"}}
	enterLine(model, "help ask")
	wide := len(model.helpLines())
	model.Update(tea.WindowSizeMsg{Width: 30, Height: 30})
	if len(model.helpLines()) <= wide {
		test.Fatal("description did not wrap on resize")
	}
	for _, line := range model.helpLines() {
		if strings.Contains(line, "\x1b[31m") {
			test.Fatal("plugin injected styling")
		}
		if lipgloss.Width(line) > 28 {
			test.Errorf("line overflows: %q", ansi.Strip(line))
		}
	}
	golden.Assert(test, "help_plugin_narrow", screen(model), false)
	// Search is shared with the other help pages.
	press(model, "/")
	for _, character := range "plugin" {
		press(model, string(character))
	}
	follow(model, press(model, "enter"))
	if !strings.Contains(screen(model), "plugin") {
		test.Fatal("search lost the help body")
	}
	press(model, "esc")
	if !model.showHelp {
		test.Fatal("first escape should clear the search")
	}
	press(model, "esc")
	if model.showHelp {
		test.Fatal("second escape should close help")
	}
}

func TestShellCommandHelpIsComplete(test *testing.T) {
	for _, definition := range commandTable() {
		if strings.TrimSpace(definition.description) == "" {
			test.Errorf("command %v has no help text", definition.names)
		}
		for _, name := range definition.names {
			model := sizedModel(120, 30)
			enterLine(model, "help "+name)
			if !model.showHelp || model.helpContent != helpCommandPage {
				test.Errorf("command %s did not open command help", name)
			}
		}
	}
}

func TestSharedCommandHelpShowsEveryView(test *testing.T) {
	model := sizedModel(100, 30)
	model.commandHelp = []CommandHelp{
		{Name: "copy-url", Description: "copy the selected row", View: "Dashboard"},
		{Name: "copy-url", Description: "copy the open pull request", View: "Pull request workspace"},
	}
	enterLine(model, "help copy-url")
	for _, want := range []string{"copy the selected row", "copy the open pull request", "Views: Dashboard", "Views: Pull request workspace"} {
		if !strings.Contains(screen(model), want) {
			test.Errorf("help lacks %q", want)
		}
	}
}
