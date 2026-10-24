package search

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// Prefix is what the bottom line shows while a search is typed. A view
// recognises its search prompt by it.
const Prefix = "/"

// List is how a view lets a search read and move its rows.
type List struct {
	// Rows returns the clean text of every row, in row order.
	Rows func() []string
	// Cursor returns the row the cursor is on.
	Cursor func() int
	// GoTo moves the cursor to a row and returns any command that follows
	// (such as loading the next page).
	GoTo func(row int) tea.Cmd
}

// Start begins a search: remember the cursor and ask the shell for the prompt.
func (state *State) Start(list List) tea.Cmd {
	state.Begin(list.Cursor())
	return userinterface.Prompt(Prefix)
}

// Prompt applies what the shell reports about the search prompt. Typing moves
// the cursor live; enter keeps the search; esc puts back the cursor and the
// previous search. Results for other prompts are ignored.
func (state *State) Prompt(result userinterface.PromptResultMessage, list List) tea.Cmd {
	if result.Prefix != Prefix {
		return nil
	}
	switch result.State {
	case userinterface.PromptChanged:
		row, ok := state.Type(result.Text, list.Rows())
		if !ok {
			row = state.Origin()
		}
		return list.GoTo(row)
	case userinterface.PromptCancelled:
		state.Cancel(list.Rows())
		return list.GoTo(state.Origin())
	case userinterface.PromptSubmitted:
		if result.Text == "" {
			state.Clear()
			return list.GoTo(state.Origin())
		}
		if _, ok := state.Type(result.Text, list.Rows()); !ok {
			return tea.Batch(list.GoTo(state.Origin()), userinterface.Info("no match: "+terminaltext.Clean(result.Text)))
		}
	}
	return nil
}

// Step handles n (forward) and N: the count-th next match, wrapping. With no
// search it does nothing; with no match it says so.
func (state *State) Step(message keys.Message, forward bool, list List) tea.Cmd {
	if !state.Active() {
		return nil
	}
	step := state.Previous
	if forward {
		step = state.Next
	}
	row, ok := step(list.Cursor(), message.Repeat())
	if !ok {
		return userinterface.Info("no match: " + terminaltext.Clean(state.Query()))
	}
	return list.GoTo(row)
}
