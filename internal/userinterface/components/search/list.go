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

// Item is one row of a list as a search sees it.
type Item struct {
	// Text is the clean text the search looks in.
	Text string
	// Identity names what the row shows (a pull request, a path) and stays
	// the same when rows load or change.
	Identity string
}

// List is how a view lets a search read and move its rows.
type List struct {
	// Items returns every row, in row order.
	Items func() []Item
	// Cursor returns the row the cursor is on.
	Cursor func() int
	// CursorKey returns the identity of the cursor row, or "" when the cursor
	// was never moved by the user. Optional; the default is the identity of
	// the item under Cursor.
	CursorKey func() string
	// GoTo moves the cursor to a row and returns any command that follows
	// (such as loading the next page).
	GoTo func(row int) tea.Cmd
	// Restore puts the cursor back on the item with identity key, found at
	// row, when a search is cancelled. Optional; the default is GoTo(row).
	Restore func(key string, row int) tea.Cmd
}

// texts returns the text of every row.
func (list List) texts() []string {
	items := list.Items()
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}
	return texts
}

// cursorKey is the identity of the row the cursor is on.
func (list List) cursorKey() string {
	if list.CursorKey != nil {
		return list.CursorKey()
	}
	items, cursor := list.Items(), list.Cursor()
	if cursor < 0 || cursor >= len(items) {
		return ""
	}
	return items[cursor].Identity
}

// Start begins a search. The shell opens the prompt when it asks the view
// (see userinterface.Prompter), in the same update as the key.
func (state *State) Start(list List) tea.Cmd {
	state.Begin(list.Cursor(), list.cursorKey())
	return nil
}

// Refresh recomputes the matches after the rows changed, keeping the query.
// Without a search it does nothing.
func (state *State) Refresh(list List) {
	if state.Active() {
		state.set(state.query, list.texts())
	}
}

// origin is the row the cursor was on when the search began, found again by
// identity because the rows may have changed since.
func (state *State) origin(list List) int {
	items := list.Items()
	if state.originKey != "" {
		if row := slices.IndexFunc(items, func(item Item) bool { return item.Identity == state.originKey }); row >= 0 {
			return row
		}
	}
	return max(min(state.originRow, len(items)-1), 0)
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
// previous search; enter on an empty prompt repeats the last search. Results for other prompts are ignored.
func (state *State) Prompt(result userinterface.PromptResultMessage, list List) tea.Cmd {
	if result.Prefix != Prefix {
		return nil
	}
	query := strings.TrimSpace(result.Text)
	switch result.State {
	case userinterface.PromptChanged:
		row, ok := state.Type(query, list.texts(), state.origin(list))
		if !ok {
			return state.restore(list)
		}
		return list.GoTo(row)
	case userinterface.PromptCancelled:
		state.Cancel(list.texts())
		return state.restore(list)
	case userinterface.PromptSubmitted:
		if query == "" {
			query = state.last // enter on an empty prompt repeats the last search
		}
		if query == "" {
			state.Clear()
			return state.restore(list)
		}
		state.last = query
		row, ok := state.Type(query, list.texts(), state.origin(list))
		if !ok {
			return tea.Batch(state.restore(list), userinterface.Info("no match: "+terminaltext.Clean(query)))
		}
		return list.GoTo(row)
	}
	return nil
}

// Step handles n (forward) and N: the count-th next match, wrapping when
// wrapScan (the wrapscan option) is on. With no search it does nothing; with no match it says
// so, and without wrapping it says when it reached the end.
func (state *State) Step(message keys.Message, forward bool, list List, wrapScan bool) tea.Cmd {
	if !state.Active() {
		return nil
	}
	step := state.Previous
	if forward {
		step = state.Next
	}
	row, ok := step(list.Cursor(), message.Repeat(), wrapScan)
	switch {
	case ok:
	case len(state.matches) == 0:
		return userinterface.Info("no match: " + terminaltext.Clean(state.Query()))
	case forward:
		return userinterface.Info("no more matches below")
	default:
		return userinterface.Info("no more matches above")
	}
	return list.GoTo(row)
}
