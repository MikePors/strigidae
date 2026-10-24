package dashboard

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

// searchList lets the tab's search read and move its rows, which are numbered
// across all sections.
func (current *tab) searchList() search.List {
	return search.List{
		Items: func() []search.Item {
			var items []search.Item
			for i, target := range current.sections {
				items = append(items, pullrequestrows.SearchItems(target.pullRequests, func(_ int, pullRequest github.PullRequest) string {
					return rowKey(i, pullRequest)
				})...)
			}
			return items
		},
		Cursor: current.selectedIndex,
		CursorKey: func() string {
			if current.anchor == nil {
				return "" // never moved: it follows the first loaded row
			}
			if pullRequest, ok := current.selectedPullRequest(); ok {
				section, _, _ := current.position()
				return rowKey(section, pullRequest)
			}
			return ""
		},
		GoTo: func(row int) tea.Cmd {
			current.selectIndex(row)
			return nil
		},
		Restore: func(key string, row int) tea.Cmd {
			if key == "" {
				current.anchor = nil
				return nil
			}
			current.selectIndex(row)
			return nil
		},
	}
}

// rowKey identifies a row by its section as well as its pull request, because
// one pull request often appears in several sections.
func rowKey(section int, pullRequest github.PullRequest) string {
	return fmt.Sprintf("%d/%s#%d", section, pullRequest.Repository, pullRequest.Number)
}
