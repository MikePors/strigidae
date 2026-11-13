package main

import (
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/views/dashboard"
	"github.com/MikePors/strigidae/internal/userinterface/views/pullrequestlist"
	"github.com/MikePors/strigidae/internal/userinterface/views/workspace"
)

// viewCommandHelp collects the registries here, where importing every view is
// allowed, so help does not depend on which views have been opened.
func viewCommandHelp() []userinterface.CommandHelp {
	var commands []userinterface.CommandHelp
	for _, group := range []struct {
		name    string
		actions []keys.Action
	}{
		{"Pull request list", pullrequestlist.Actions()},
		{"Dashboard", dashboard.Actions()},
		{"Pull request workspace", workspace.Actions()},
		{"Diff pane", workspace.DiffActions()},
	} {
		for _, action := range group.actions {
			if action.Command == "" {
				continue
			}
			command := userinterface.CommandHelp{Name: action.Command, Arguments: action.Arguments, Description: action.Help(), View: group.name}
			if action.Binding != nil {
				command.Keys = action.Binding.Keys()
			}
			commands = append(commands, command)
		}
	}
	return commands
}
