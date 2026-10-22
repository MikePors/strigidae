package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// ConfirmMsg asks the root to show a yes/no question in the status row. The
// root appends " y/N" to Text. While it shows, the root captures every key:
// `y` runs OnYes, anything else (esc included) cancels. A view returns it for
// actions that must not happen by accident, such as posting a comment.
type ConfirmMsg struct {
	// Text is the question, already cleaned of untrusted escapes.
	Text string
	// OnYes runs when the user answers yes.
	OnYes tea.Cmd
}

// Confirm returns a command that asks the question.
func Confirm(text string, onYes tea.Cmd) tea.Cmd {
	return msgCmd(ConfirmMsg{Text: text, OnYes: onYes})
}

// confirmPrompt is the open question. It implements KeyCapturer.
type confirmPrompt struct {
	text  string
	onYes tea.Cmd
	yes   key.Binding
}

var _ KeyCapturer = (*confirmPrompt)(nil)

func newConfirmPrompt(msg ConfirmMsg) *confirmPrompt {
	return &confirmPrompt{
		text:  msg.Text,
		onYes: msg.OnYes,
		yes:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
	}
}

// CapturesKeys implements KeyCapturer: a nil prompt captures nothing.
func (c *confirmPrompt) CapturesKeys() bool { return c != nil }

// answer returns the command for a key press: OnYes for `y`, nil otherwise.
func (c *confirmPrompt) answer(msg tea.KeyPressMsg) tea.Cmd {
	if key.Matches(msg, c.yes) {
		return c.onYes
	}
	return nil
}

// render fits the question and the choices in width columns; the choices
// always stay visible.
func (c *confirmPrompt) render(width int) string {
	const choices = " y/N"
	text := " " + strings.ReplaceAll(ansi.Strip(c.text), "\n", " ")
	return ansi.Truncate(text, max(0, width-len(choices)), "…") + choices
}
