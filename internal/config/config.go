// Package config loads strig's YAML config file: tabs made of sections, each
// section a GitHub search query. It is a non-UI package; cmd/strig loads the
// config and passes it in.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Config is the validated, fully resolved configuration.
type Config struct {
	// Tabs are shown in order; there is always at least one.
	Tabs []Tab
}

// Tab is a named group of sections, for example the viewer's own PRs or a
// teammate's.
type Tab struct {
	Name string
	// User is the GitHub login (or "@me") the sections were templated with.
	User     string
	Sections []Section
}

// Section is one PR list driven by a GitHub search query.
type Section struct {
	Name string
	// Query is the final search query, with the tab's user already
	// substituted for {user}.
	Query string
	// Limit is the most PRs to fetch, 1 to MaxLimit.
	Limit int
	Sort  Sort
	Order Order
}

// Path returns the config file location: $XDG_CONFIG_HOME/strig/config.yml,
// falling back to ~/.config/strig/config.yml. getenv and home are injected
// for tests.
func Path(getenv func(string) string, home func() (string, error)) (string, error) {
	if dir := getenv("XDG_CONFIG_HOME"); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "strig", "config.yml"), nil
	}
	h, err := home()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(h, ".config", "strig", "config.yml"), nil
}

// Load reads and validates the file at path. A missing file yields Default
// unless mustExist is set (the user named the file explicitly, so a typo
// shouldn't silently fall back). Problems in the file are returned as *Error.
func Load(path string, mustExist bool) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !mustExist {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return Parse(path, data)
}
