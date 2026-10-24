package dashboard

import (
	"testing"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestScreenPositioningMovesTheViewNotTheSelection(test *testing.T) {
	configured := config.Section{Name: "s", Query: "is:pr s", Limit: 30, Sort: config.SortUpdated, Order: config.OrderDescending}
	tabs := []config.Tab{{Name: "T", Sections: []config.Section{configured}}}
	var rows []github.PullRequest
	for i := range 30 {
		rows = append(rows, pullRequest("o/r", i+1, "Row", 0))
	}
	client := fake.New()
	client.SetSearchResults(searchQuery(configured), rows)
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	for range 14 { // line 15: the heading is line 0
		press(view, "j")
		view.Render(80, 12) // 10 list rows
	}
	selected := selectedKey(test, view)
	tests := []struct {
		key     string
		wantTop int
	}{{"z t", 15}, {"z z", 11}, {"z b", 6}}
	for _, testCase := range tests {
		press(view, testCase.key)
		view.Render(80, 12)
		if got := selectedKey(test, view); got != selected {
			test.Errorf("%s moved the selection from %s to %s", testCase.key, selected, got)
		}
		if got := view.tabs[view.active].window.Top(); got != testCase.wantTop {
			test.Errorf("%s: top = %d, want %d", testCase.key, got, testCase.wantTop)
		}
	}
}
