package userinterface

import (
	"strings"

	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// CommandHelp describes a command in one view. Commands shared by views have
// one entry per view, preserving each view's description and bindings.
type CommandHelp struct {
	// Name is the command or alias, without its leading colon.
	Name string
	// Arguments is the usage suffix, including optional or required markers.
	Arguments string
	// Description is the help text from the command's registry.
	Description string
	// Keys lists the key sequences that invoke this command's action.
	Keys []string
	// View names the view in which the command works.
	View string
}

func (model *Model) allCommandHelp() []CommandHelp {
	var commands []CommandHelp
	for _, definition := range commandTable() {
		for _, name := range definition.names {
			commands = append(commands, CommandHelp{Name: name, Arguments: definition.arguments, Description: definition.description, View: "Every view"})
		}
	}
	commands = append(commands, model.commandHelp...)
	for _, plugin := range model.pluginCommands {
		description := plugin.Description
		if strings.TrimSpace(description) == "" {
			description = "run the configured plugin on " + string(plugin.Target)
		}
		commands = append(commands, CommandHelp{Name: plugin.Name, Arguments: "[arguments]", Description: description, View: "Pull request workspace"})
	}
	return commands
}

func (model *Model) commandHelpLines(width int) []string {
	var lines []string
	for _, command := range model.allCommandHelp() {
		if command.Name != model.helpCommandName {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		usage := ":" + command.Name
		if command.Arguments != "" {
			usage += " " + command.Arguments
		}
		for _, line := range wrapWords(terminaltext.Clean(usage), width) {
			lines = append(lines, model.theme.HelpKey.Render(line))
		}
		lines = append(lines, "")
		for _, line := range wrapWords(terminaltext.Clean(command.Description), width) {
			lines = append(lines, model.theme.HelpDescription.Render(line))
		}
		bound := "none"
		if len(command.Keys) > 0 {
			bound = strings.Join(command.Keys, ", ")
		}
		for _, text := range []string{"Keys: " + bound, "Views: " + command.View} {
			for _, line := range wrapWords(terminaltext.Clean(text), width) {
				lines = append(lines, model.theme.HelpDescription.Render(line))
			}
		}
	}
	return lines
}
