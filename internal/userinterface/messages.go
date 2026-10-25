package userinterface

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// PushMessage asks the root to push View on top of the stack.
type PushMessage struct{ View View }

// PopMessage asks the root to pop the active view; on the root view it quits.
type PopMessage struct{}

// QuitMessage asks the root to exit the program.
type QuitMessage struct{}

// InfoMessage shows a transient informational message in the status bar.
type InfoMessage struct{ Text string }

// ErrorMessage shows a transient error in the status bar and logs it.
type ErrorMessage struct{ Err error }

// OptionsMessage tells views the `:set` options changed. The root also sends
// it to the root view when strig starts and to every view it pushes, so a
// view always knows the current values. Until then a view has the defaults.
type OptionsMessage struct{ Values options.Values }

// ThemeMessage tells views the theme changed (after the terminal background
// was detected, or `:theme`).
type ThemeMessage struct{ Theme theme.Theme }

// Push returns a command that pushes view.
func Push(view View) tea.Cmd { return messageCommand(PushMessage{View: view}) }

// Pop returns a command that pops the active view.
func Pop() tea.Cmd { return messageCommand(PopMessage{}) }

// Quit returns a command that exits the program.
func Quit() tea.Cmd { return messageCommand(QuitMessage{}) }

// Info returns a command that shows an info message.
func Info(text string) tea.Cmd { return messageCommand(InfoMessage{Text: text}) }

// Error returns a command that shows an error message.
func Error(err error) tea.Cmd { return messageCommand(ErrorMessage{Err: err}) }

func messageCommand(message tea.Msg) tea.Cmd { return func() tea.Msg { return message } }
