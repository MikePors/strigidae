package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

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

// refreshSearch recomputes the marks after the files changed.
func (view *View) refreshSearch() {
	if view.search.Active() {
		view.search.Refresh(view.searchList().Rows())
	}
}
