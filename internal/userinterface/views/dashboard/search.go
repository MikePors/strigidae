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
		Rows: func() []string {
			pullRequests := current.pullRequests()
			rows := make([]string, len(pullRequests))
			for i, pullRequest := range pullRequests {
				rows[i] = pullrequestrows.SearchText(pullRequest)
			}
			return rows
		},
		Identities: func() []string {
			identities := make([]string, 0, current.total())
			for i, target := range current.sections {
				for _, pullRequest := range target.pullRequests {
					identities = append(identities, rowKey(i, pullRequest))
				}
			}
			return identities
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

// refreshSearch recomputes the marks after the rows changed.
func (current *tab) refreshSearch() {
	if current.search.Active() {
		current.search.Refresh(current.searchList().Rows())
	}
}
