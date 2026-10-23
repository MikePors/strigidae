package userinterface

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// ConfirmMsg asks the root to show a yes/no question in the status row. The
// root appends " y/N" to Text. While it shows, the root captures every key:
// `y` runs OnYes, anything else (esc included) cancels. A view returns it for
// actions that must not happen by accident, such as posting a comment.
//
// A ConfirmMsg that arrives while another prompt is open replaces it
// silently (the visible text always matches the action that would run). Views
// whose confirmation arrives after a network round trip should expect that.
type ConfirmMessage struct {
	// Text is the question, already cleaned of untrusted escapes.
	Text string
	// OnYes runs when the user answers yes.
	OnYes tea.Cmd
	// Notice turns the question into a notice: no y/N is offered, and any key
	// dismisses it and runs OnDismiss (OnYes is never used in notice mode).
	Notice bool
	// OnDismiss runs on any key when Notice is set; nil is fine.
	OnDismiss tea.Cmd
}

// Confirm returns a command that asks the question.
func Confirm(text string, onYes tea.Cmd) tea.Cmd {
	return messageCommand(ConfirmMessage{Text: text, OnYes: onYes})
}

// confirmPrompt is the open question. It implements KeyCapturer.
type confirmPrompt struct {
	text      string
	onYes     tea.Cmd
	yes       key.Binding
	notice    bool
	onDismiss tea.Cmd
}

var _ KeyCapturer = (*confirmPrompt)(nil)

func newConfirmPrompt(message ConfirmMessage) *confirmPrompt {
	return &confirmPrompt{
		text:      message.Text,
		onYes:     message.OnYes,
		yes:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		notice:    message.Notice,
		onDismiss: message.OnDismiss,
	}
}

// CapturesKeys implements KeyCapturer: a nil prompt captures nothing.
func (confirmPrompt *confirmPrompt) CapturesKeys() bool { return confirmPrompt != nil }

// answer returns the command for a key press: OnYes for `y`, nil otherwise.
func (confirmPrompt *confirmPrompt) answer(message tea.KeyPressMsg) tea.Cmd {
	if confirmPrompt.notice {
		return confirmPrompt.onDismiss
	}
	if key.Matches(message, confirmPrompt.yes) {
		return confirmPrompt.onYes
	}
	return nil
}

// render fits the question and the choices in width columns; the choices
// always stay visible.
func (confirmPrompt *confirmPrompt) render(width int) string {
	choices := " y/N"
	if confirmPrompt.notice {
		choices = " (any key)"
	}
	// Tabs and other controls would be measured differently than drawn.
	text := " " + strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return ' '
		}
		return character
	}, ansi.Strip(confirmPrompt.text))
	return ansi.Truncate(text, max(0, width-len(choices)), "…") + choices
}
