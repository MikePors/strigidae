package pullrequestlist

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

// searchList lets the search read and move the shown tab's rows.
func (view *View) searchList() search.List { return view.searchListOf(view.current()) }

// searchListOf is searchList for any tab, so rows arriving in a hidden tab can
// refresh its search marks.
func (view *View) searchListOf(current *tab) search.List {
	return search.List{
		Items: func() []search.Item {
			return pullrequestrows.SearchItems(current.pullRequests, func(_ int, pullRequest github.PullRequest) string {
				return strconv.Itoa(pullRequest.Number)
			})
		},
		Cursor: current.window.Cursor,
		GoTo: func(row int) tea.Cmd {
			current.window.GotoRow(row)
			return view.maybeLoadMore()
		},
	}
}

// PromptRequest implements userinterface.Prompter.
func (view *View) PromptRequest() (prefix string, ok bool) {
	return search.Prefix, view.current().search.TakeRequest()
}
