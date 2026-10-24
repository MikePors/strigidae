package userinterface

import (
	tea "charm.land/bubbletea/v2"
)

// PromptState says what happened to a prompt the shell opened for a view.
type PromptState int

// The states a prompt reports.
const (
	PromptChanged   PromptState = iota // the text changed while typing
	PromptSubmitted                    // enter: the prompt is closed and Text is final
	PromptCancelled                    // esc, or backspace on an empty line: the prompt is closed
)

// PromptMessage asks the shell to open the bottom line, where ":" commands are
// typed, as a prompt that starts with Prefix. The shell owns the input and the
// key capture; the top view hears about it through PromptResultMessage.
type PromptMessage struct{ Prefix string }

// PromptResultMessage reports a prompt to the top view: on every change of the
// text, on enter and on esc. Prefix tells the view which prompt it is.
type PromptResultMessage struct {
	Prefix string
	Text   string
	State  PromptState
}

// Prompt returns a command that opens a prompt in the bottom line.
func Prompt(prefix string) tea.Cmd { return messageCommand(PromptMessage{Prefix: prefix}) }

// openPrompt turns the command line into a prompt that starts with prefix.
func (commandLine *commandLine) openPrompt(prefix string) tea.Cmd {
	command := commandLine.open()
	commandLine.input.Prompt = prefix
	commandLine.prompt = prefix
	return command
}

// promptKey handles one key while the command line is a prompt, reporting to
// the top view. The line closes before the view hears about it, so capture is
// already released when the view reacts.
func (model *Model) promptKey(message tea.KeyPressMsg) tea.Cmd {
	prefix, before := model.commandLine.prompt, model.commandLine.input.Value()
	finish := func(state PromptState) tea.Cmd {
		text := model.commandLine.input.Value()
		model.commandLine.close()
		return model.forward(PromptResultMessage{Prefix: prefix, Text: text, State: state})
	}
	switch message.String() {
	case "esc":
		return finish(PromptCancelled)
	case "enter":
		return finish(PromptSubmitted)
	case "backspace":
		if before == "" {
			return finish(PromptCancelled)
		}
	}
	blink := model.commandLine.update(message)
	if after := model.commandLine.input.Value(); after != before {
		return tea.Batch(blink, model.forward(PromptResultMessage{Prefix: prefix, Text: after, State: PromptChanged}))
	}
	return blink
}
