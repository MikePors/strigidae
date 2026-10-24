package pullrequestlist

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

// searchList lets the search read and move the rows.
func (view *View) searchList() search.List {
	return search.List{
		Rows: func() []string {
			rows := make([]string, len(view.pullRequests))
			for i, pullRequest := range view.pullRequests {
				rows[i] = pullrequestrows.SearchText(pullRequest)
			}
			return rows
		},
		Cursor: view.window.Cursor,
		GoTo: func(row int) tea.Cmd {
			view.window.GotoRow(row)
			return view.maybeLoadMore()
		},
	}
}

// refreshSearch recomputes the marks after the rows changed.
func (view *View) refreshSearch() {
	if view.search.Active() {
		view.search.Refresh(view.searchList().Rows())
	}
}
