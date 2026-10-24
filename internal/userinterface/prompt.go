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

// Prompter is optionally implemented by views that ask the shell for a prompt
// in the bottom line, where ":" commands are typed. After forwarding a key to
// the top view the shell calls PromptRequest; when it reports ok, the shell
// opens the prompt starting with prefix in that same update, so no key typed
// next can reach the view first. The view then hears about the prompt through
// PromptResultMessage. A request made while the command line is open is
// ignored.
type Prompter interface {
	PromptRequest() (prefix string, ok bool)
}

// PromptResultMessage reports a prompt to the top view: on every change of the
// text (typing or pasting), on enter and on esc. Prefix tells the view which
// prompt it is.
type PromptResultMessage struct {
	Prefix string
	Text   string
	State  PromptState
}

// openPrompt turns the command line into a prompt that starts with prefix.
func (commandLine *commandLine) openPrompt(prefix string) tea.Cmd {
	command := commandLine.open()
	commandLine.input.Prompt = prefix
	commandLine.prompt = prefix
	return command
}

// withRequestedPrompt opens the prompt the top view asked for while handling
// a key, alongside command.
func (model *Model) withRequestedPrompt(command tea.Cmd) tea.Cmd {
	prompter, ok := model.top().(Prompter)
	if !ok {
		return command
	}
	prefix, requested := prompter.PromptRequest()
	if !requested || model.commandLine.active {
		return command
	}
	return tea.Batch(command, model.commandLine.openPrompt(prefix))
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
	return model.promptInput(message)
}

// promptInput gives a key or a paste to the input and reports a change of its
// text to the top view.
func (model *Model) promptInput(message tea.Msg) tea.Cmd {
	prefix, before := model.commandLine.prompt, model.commandLine.input.Value()
	command := model.commandLine.update(message)
	if after := model.commandLine.input.Value(); after != before {
		return tea.Batch(command, model.forward(PromptResultMessage{Prefix: prefix, Text: after, State: PromptChanged}))
	}
	return command
}
