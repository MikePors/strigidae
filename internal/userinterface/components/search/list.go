package search

import (
	"slices"
	"strings"

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
	// Identities returns one identity per row (a pull request, a path) that
	// stays the same when rows load or change.
	Identities func() []string
	// Cursor returns the row the cursor is on.
	Cursor func() int
	// CursorKey returns the identity of the cursor row, or "" when the cursor
	// was never moved by the user.
	CursorKey func() string
	// GoTo moves the cursor to a row and returns any command that follows
	// (such as loading the next page).
	GoTo func(row int) tea.Cmd
	// Restore puts the cursor back on the item with identity key, found at
	// row, when a search is cancelled. Optional; the default is GoTo(row).
	Restore func(key string, row int) tea.Cmd
}

// Start begins a search. The shell opens the prompt when it asks the view
// (see userinterface.Prompter), in the same update as the key.
func (state *State) Start(list List) tea.Cmd {
	state.Begin(list.Cursor(), list.CursorKey())
	return nil
}

// origin is the row the cursor was on when the search began, found again by
// identity because the rows may have changed since.
func (state *State) origin(list List) int {
	if state.originKey != "" {
		if row := slices.Index(list.Identities(), state.originKey); row >= 0 {
			return row
		}
	}
	return max(min(state.originRow, len(list.Rows())-1), 0)
}

// restore puts the cursor back where the search began.
func (state *State) restore(list List) tea.Cmd {
	row := state.origin(list)
	if list.Restore != nil {
		return list.Restore(state.originKey, row)
	}
	return list.GoTo(row)
}

// Prompt applies what the shell reports about the search prompt. Typing moves
// the cursor live; enter keeps the search; esc puts back the cursor and the
// previous search. Results for other prompts are ignored.
func (state *State) Prompt(result userinterface.PromptResultMessage, list List) tea.Cmd {
	if result.Prefix != Prefix {
		return nil
	}
	query := strings.TrimSpace(result.Text)
	switch result.State {
	case userinterface.PromptChanged:
		row, ok := state.Type(query, list.Rows(), state.origin(list))
		if !ok {
			return state.restore(list)
		}
		return list.GoTo(row)
	case userinterface.PromptCancelled:
		state.Cancel(list.Rows())
		return state.restore(list)
	case userinterface.PromptSubmitted:
		if query == "" {
			state.Clear()
			return state.restore(list)
		}
		row, ok := state.Type(query, list.Rows(), state.origin(list))
		if !ok {
			return tea.Batch(state.restore(list), userinterface.Info("no match: "+terminaltext.Clean(query)))
		}
		return list.GoTo(row)
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
