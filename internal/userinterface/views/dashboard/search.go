package dashboard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

// searchList lets the tab's search read and move its rows, which are numbered
// across all sections.
func (current *tab) searchList() search.List {
	return search.List{
		Rows: func() []string {
			pullRequests := current.pullRequests()
			rows := make([]string, len(pullRequests))
			for i, pullRequest := range pullRequests {
				rows[i] = pullrequestrows.SearchText(pullRequest)
			}
			return rows
		},
		Cursor: current.selectedIndex,
		GoTo: func(row int) tea.Cmd {
			current.selectIndex(row)
			return nil
		},
	}
}

// refreshSearch recomputes the marks after the rows changed.
func (current *tab) refreshSearch() {
	if current.search.Active() {
		current.search.Refresh(current.searchList().Rows())
	}
}
