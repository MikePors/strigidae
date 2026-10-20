// Package logging sets up the file logger. The TUI owns stdout and stderr, so
// logs only ever go to a file under the XDG state directory.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Path returns the log file location: $XDG_STATE_HOME/strig/strig.log, falling
// back to ~/.local/state/strig/strig.log. getenv and home are injected for tests.
func Path(getenv func(string) string, home func() (string, error)) (string, error) {
	if dir := getenv("XDG_STATE_HOME"); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "strig", "strig.log"), nil
	}
	h, err := home()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(h, ".local", "state", "strig", "strig.log"), nil
}

// Open creates the log file (and its directory) and returns a logger writing
// to it. The caller must close the file.
func Open(path string) (*slog.Logger, *os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	return slog.New(slog.NewTextHandler(f, nil)), f, nil
}

// Install routes the standard library's default loggers to l, so a
// dependency calling log.Printf or slog.Info can never write to stderr and
// corrupt the screen. slog.SetDefault also sends the log package through l's
// handler, so those lines get a timestamp and level. This is the one place the
// program mutates global logger state; pass a logger with an io.Discard
// handler to silence them.
func Install(l *slog.Logger) {
	slog.SetDefault(l)
}
