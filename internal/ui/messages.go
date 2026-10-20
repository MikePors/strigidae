package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/ui/theme"
)

// PushMsg asks the root to push View on top of the stack.
type PushMsg struct{ View View }

// PopMsg asks the root to pop the active view; on the root view it quits.
type PopMsg struct{}

// QuitMsg asks the root to exit the program.
type QuitMsg struct{}

// InfoMsg shows a transient informational message in the status bar.
type InfoMsg struct{ Text string }

// ErrorMsg shows a transient error in the status bar and logs it.
type ErrorMsg struct{ Err error }

// ThemeMsg tells views the theme changed (for example after the terminal
// background was detected).
type ThemeMsg struct{ Theme theme.Theme }

// Push returns a command that pushes v.
func Push(v View) tea.Cmd { return msgCmd(PushMsg{View: v}) }

// Pop returns a command that pops the active view.
func Pop() tea.Cmd { return msgCmd(PopMsg{}) }

// Quit returns a command that exits the program.
func Quit() tea.Cmd { return msgCmd(QuitMsg{}) }

// Info returns a command that shows an info message.
func Info(text string) tea.Cmd { return msgCmd(InfoMsg{Text: text}) }

// Error returns a command that shows an error message.
func Error(err error) tea.Cmd { return msgCmd(ErrorMsg{Err: err}) }

func msgCmd(m tea.Msg) tea.Cmd { return func() tea.Msg { return m } }
