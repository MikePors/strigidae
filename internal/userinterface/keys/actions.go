package keys

import (
	"fmt"
	"regexp"

	"charm.land/bubbles/v2/key"
)

// Action is a binding with its action name: the stable name a config file
// uses to refer to it, independent of its keys and help text. Names are
// lower-case words joined by hyphens, like command names ("copy-filename").
// Binding points at the field that holds it, so it can be changed in place.
type Action struct {
	Name    string
	Binding *key.Binding
}

// Actions returns the global bindings with their action names. Count is left
// out: it only documents the count prefix and has no keys of its own.
func (global *Global) Actions() []Action {
	return []Action{
		{"down", &global.Down},
		{"up", &global.Up},
		{"top", &global.Top},
		{"bottom", &global.Bottom},
		{"half-page-down", &global.HalfDown},
		{"half-page-up", &global.HalfUp},
		{"page-down", &global.PageDown},
		{"page-up", &global.PageUp},
		{"scroll-cursor-top", &global.ScrollCursorTop},
		{"scroll-cursor-middle", &global.ScrollCursorMiddle},
		{"scroll-cursor-bottom", &global.ScrollCursorBottom},
		{"select", &global.Select},
		{"open-in-browser", &global.Open},
		{"refresh", &global.Refresh},
		{"search", &global.Search},
		{"next-match", &global.NextMatch},
		{"previous-match", &global.PreviousMatch},
		{"cancel", &global.Cancel},
		{"help", &global.Help},
		{"command-line", &global.Command},
	}
}

var actionName = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)

// CheckActions reports the first action whose name is not lower-case words
// joined by hyphens, or is used twice. Names share one namespace across the
// global bindings and every view's, so a view's tests pass its actions
// together with the global ones.
func CheckActions(actions []Action) error {
	seen := map[string]bool{}
	for _, action := range actions {
		if !actionName.MatchString(action.Name) {
			return fmt.Errorf("action name %q is not lower-case words joined by hyphens", action.Name)
		}
		if seen[action.Name] {
			return fmt.Errorf("action name %q is used twice", action.Name)
		}
		seen[action.Name] = true
	}
	return nil
}
