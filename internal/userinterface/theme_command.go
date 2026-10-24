package userinterface

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/theme/scheme"
)

// ThemeLoader loads the theme a `:theme` value names. It runs inside a
// tea.Cmd, so it may read files.
type ThemeLoader func(value string) (scheme.Scheme, error)

// LoadThemeFromWorkingDirectory resolves value the way the config's `theme:`
// is resolved, except that a relative path is taken from the current
// directory.
func LoadThemeFromWorkingDirectory(value string) (scheme.Scheme, error) {
	directory, err := os.Getwd()
	if err != nil {
		return scheme.Scheme{}, fmt.Errorf("find the current directory: %w", err)
	}
	return scheme.Resolve(value, directory, os.UserHomeDir)
}

// themeRequestedMessage asks the root to load and switch to a theme (`:theme`).
type themeRequestedMessage struct{ value string }

// themeLoadedMessage carries a loaded theme back. sequence discards a load
// that a later `:theme` superseded.
type themeLoadedMessage struct {
	sequence int
	value    string
	scheme   scheme.Scheme
	err      error
}

// themeCommand is `:theme <value>`.
func themeCommand(arguments string) tea.Cmd {
	if arguments == "" {
		return Error(errors.New("usage: :theme <name or file>"))
	}
	if strings.EqualFold(arguments, scheme.Terminal) {
		// The terminal is asked only before the screen appears, so there is
		// nothing to load while strig runs.
		return Info("theme terminal can only be set in the config (it's read when strig starts)")
	}
	return messageCommand(themeRequestedMessage{value: arguments})
}

// loadTheme reads the theme in a command, off the UI goroutine.
func (model *Model) loadTheme(value string) tea.Cmd {
	model.themeSequence++
	sequence, load := model.themeSequence, model.themeLoader
	return func() tea.Msg {
		loaded, err := load(value)
		return themeLoadedMessage{sequence: sequence, value: value, scheme: loaded, err: err}
	}
}

// themeLoaded switches to a loaded theme, or reports why it could not load
// and keeps the current one. The theme is built here, on the UI goroutine,
// because building styles may register syntax styles.
func (model *Model) themeLoaded(message themeLoadedMessage) tea.Cmd {
	if message.sequence != model.themeSequence {
		return nil
	}
	if message.err != nil {
		// The error already names the theme or file. It may quote the typed
		// value or the file's contents, so it is cleaned like other text.
		return Error(errors.New(terminaltext.Clean(message.err.Error())))
	}
	for _, warning := range message.scheme.Warnings {
		model.log.Warn("theme", "name", message.scheme.Name, "warning", warning)
	}
	return tea.Batch(model.setTheme(message.scheme.Theme()), Info("theme: "+terminaltext.Clean(message.scheme.Name)))
}

// setTheme makes activeTheme the theme everything draws with and tells every
// view.
func (model *Model) setTheme(activeTheme theme.Theme) tea.Cmd {
	model.theme = activeTheme
	model.commandLine.setTheme(activeTheme)
	model.spin.Style = activeTheme.Spinner
	return model.broadcast(ThemeMessage{Theme: activeTheme})
}
