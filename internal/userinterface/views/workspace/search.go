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
		Rows: func() []string {
			rows := make([]string, view.entries())
			for i, file := range view.files() {
				rows[i+1] = terminaltext.Clean(file.Path)
			}
			return rows
		},
		Identities: func() []string {
			identities := make([]string, view.entries())
			identities[0] = overviewKey
			for i, file := range view.files() {
				identities[i+1] = fileKeyPrefix + file.Path
			}
			return identities
		},
		Cursor: view.window.Cursor,
		CursorKey: func() string {
			if view.selection() == 0 || view.selection() > len(view.files()) {
				return overviewKey
			}
			return fileKeyPrefix + view.files()[view.selection()-1].Path
		},
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

// refreshSearch recomputes the marks after the files changed.
func (view *View) refreshSearch() {
	if view.search.Active() {
		view.search.Refresh(view.searchList().Rows())
	}
}
