package keys

import "charm.land/bubbles/v2/key"

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
