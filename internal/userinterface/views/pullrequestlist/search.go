package pullrequestlist

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

// searchList lets the search read and move the rows.
func (view *View) searchList() search.List {
	return search.List{
		Items: func() []search.Item {
			return pullrequestrows.SearchItems(view.pullRequests, func(_ int, pullRequest github.PullRequest) string {
				return strconv.Itoa(pullRequest.Number)
			})
		},
		Cursor: view.window.Cursor,
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
