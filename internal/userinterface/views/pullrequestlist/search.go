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
			return pullrequestrows.SearchItems(view.current().pullRequests, func(_ int, pullRequest github.PullRequest) string {
				return strconv.Itoa(pullRequest.Number)
			})
		},
		Cursor: view.current().window.Cursor,
		GoTo: func(row int) tea.Cmd {
			view.current().window.GotoRow(row)
			return view.maybeLoadMore()
		},
	}
}

// PromptRequest implements userinterface.Prompter.
func (view *View) PromptRequest() (prefix string, ok bool) {
	return search.Prefix, view.current().search.TakeRequest()
}
