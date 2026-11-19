package main

import (
	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

// globalKeys applies the config's keys: section to the default global
// bindings. It returns nil when the file overrides none, so the shell keeps
// the defaults. An unknown action or a key that can't be written is an error
// at the line of the action, like the other config errors.
func globalKeys(path string, overrides []config.KeyOverride) (*keys.Global, error) {
	if len(overrides) == 0 {
		return nil, nil
	}
	global := keys.DefaultGlobal()
	for _, override := range overrides {
		if err := global.Override(override.Action, override.Keys); err != nil {
			return nil, &config.Error{Path: path, Line: override.Line, Message: err.Error()}
		}
	}
	return &global, nil
}
