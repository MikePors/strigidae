package workspace

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// Identities of Files pane rows: the paths are prefixed so no file can look
// like Overview.
const (
	overviewKey   = "overview"
	fileKeyPrefix = "file:"
)

// PromptRequest implements userinterface.Prompter.
func (view *View) PromptRequest() (prefix string, ok bool) {
	return search.Prefix, view.search.TakeRequest()
}

// searchHelp lists the search keys, which act only with the Files pane
// focused, under help text that says so.
func (view *View) searchHelp() []key.Binding {
	searchKey, next, previous := view.globalKeys.Search, view.globalKeys.NextMatch, view.globalKeys.PreviousMatch
	searchKey.SetHelp("/", "search the Files pane")
	next.SetHelp("n", "next Files pane match")
	previous.SetHelp("N", "previous Files pane match")
	return []key.Binding{searchKey, next, previous}
}

// searchList lets the search read and move the Files pane rows. Row 0 is
// Overview, which has no text to match.
func (view *View) searchList() search.List {
	return search.List{
		Items: func() []search.Item {
			items := []search.Item{{Identity: overviewKey}}
			for _, file := range view.files() {
				items = append(items, search.Item{Text: terminaltext.Clean(file.Path), Identity: fileKeyPrefix + file.Path})
			}
			return items
		},
		Cursor: view.window.Cursor,
		GoTo: func(row int) tea.Cmd {
			previous := view.selection()
			view.window.GotoRow(row)
			if view.selection() != previous {
				view.selectionChanged()
			}
			view.layout()
			return nil
		},
	}
}
