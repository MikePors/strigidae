package pullrequestlist

import (
	"strconv"

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
		Identities: func() []string {
			identities := make([]string, len(view.pullRequests))
			for i, pullRequest := range view.pullRequests {
				identities[i] = strconv.Itoa(pullRequest.Number)
			}
			return identities
		},
		Cursor: view.window.Cursor,
		CursorKey: func() string {
			if pullRequest, ok := view.selected(); ok {
				return strconv.Itoa(pullRequest.Number)
			}
			return ""
		},
		GoTo: func(row int) tea.Cmd {
			view.window.GotoRow(row)
			return view.maybeLoadMore()
		},
	}
}

// PromptRequest implements userinterface.Prompter.
func (view *View) PromptRequest() (prefix string, ok bool) {
	return search.Prefix, view.search.TakeRequest()
}

// refreshSearch recomputes the marks after the rows changed.
func (view *View) refreshSearch() {
	if view.search.Active() {
		view.search.Refresh(view.searchList().Rows())
	}
}
