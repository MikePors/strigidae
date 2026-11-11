package keys

import (
	"fmt"
	"regexp"

	"charm.land/bubbles/v2/key"
)

// Action is a binding with its action name: the stable name a config file
// uses to refer to it, independent of its keys and help text. Names are
// lower-case words joined by hyphens, like command names ("copy-filename").
// Binding points at the field that holds it, so it can be changed in place;
// it is nil for an action that has a command and no keys. Command is the name
// of the ":" command that runs the same action, without the colon, or empty.
// A command name may equal the action's own name. Description is the help
// text of an action without a binding; one with a binding is described by the
// binding's own help (see Help).
type Action struct {
	Name        string
	Binding     *key.Binding
	Command     string
	Description string
}

// Help returns the one-line description of the action: the help text of its
// binding, or Description when it has none.
func (action Action) Help() string {
	if action.Binding != nil && action.Binding.Help().Desc != "" {
		return action.Binding.Help().Desc
	}
	return action.Description
}

// Actions returns the global bindings with their action names. Count is left
// out: it only documents the count prefix and has no keys of its own.
func (global *Global) Actions() []Action {
	return []Action{
		{Name: "down", Binding: &global.Down},
		{Name: "up", Binding: &global.Up},
		{Name: "top", Binding: &global.Top},
		{Name: "bottom", Binding: &global.Bottom},
		{Name: "half-page-down", Binding: &global.HalfDown},
		{Name: "half-page-up", Binding: &global.HalfUp},
		{Name: "page-down", Binding: &global.PageDown},
		{Name: "page-up", Binding: &global.PageUp},
		{Name: "scroll-cursor-top", Binding: &global.ScrollCursorTop},
		{Name: "scroll-cursor-middle", Binding: &global.ScrollCursorMiddle},
		{Name: "scroll-cursor-bottom", Binding: &global.ScrollCursorBottom},
		{Name: "select", Binding: &global.Select},
		{Name: "open-in-browser", Binding: &global.Open},
		{Name: "refresh", Binding: &global.Refresh},
		{Name: "search", Binding: &global.Search},
		{Name: "next-match", Binding: &global.NextMatch},
		{Name: "previous-match", Binding: &global.PreviousMatch},
		{Name: "cancel", Binding: &global.Cancel},
		{Name: "help", Binding: &global.Help},
		{Name: "command-line", Binding: &global.Command},
	}
}

var actionName = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)

// CheckActions reports the first action whose name or command is not
// lower-case words joined by hyphens, or is used twice (a command may share
// its action's name, but no two actions share a command). Names share one namespace across the
// global bindings and every view's, so a view's tests pass its actions
// together with the global ones.
func CheckActions(actions []Action) error {
	seen, seenCommand := map[string]bool{}, map[string]bool{}
	for _, action := range actions {
		if !actionName.MatchString(action.Name) {
			return fmt.Errorf("action name %q is not lower-case words joined by hyphens", action.Name)
		}
		if seen[action.Name] {
			return fmt.Errorf("action name %q is used twice", action.Name)
		}
		seen[action.Name] = true
		if action.Command == "" {
			continue
		}
		if !actionName.MatchString(action.Command) {
			return fmt.Errorf("command name %q is not lower-case words joined by hyphens", action.Command)
		}
		if seenCommand[action.Command] {
			return fmt.Errorf("command name %q is used twice", action.Command)
		}
		seenCommand[action.Command] = true
	}
	return nil
}
