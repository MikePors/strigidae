package dashboard

import (
	"testing"
	"time"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

// countsConfig has four tabs, the first with four sections of two rows each,
// so counts of two and three are visibly different from one step.
const countsConfig = `tabs:
  - name: First
    sections:
      - {name: One, query: 'is:pr one', limit: 5}
      - {name: Two, query: 'is:pr two', limit: 5}
      - {name: Three, query: 'is:pr three', limit: 5}
      - {name: Four, query: 'is:pr four', limit: 5}
  - name: Second
    sections:
      - {name: Only, query: 'is:pr only', limit: 5}
  - name: Third
    sections:
      - {name: Only, query: 'is:pr only', limit: 5}
  - name: Fourth
    sections:
      - {name: Only, query: 'is:pr only', limit: 5}
`

func countsView(test *testing.T) *View {
	test.Helper()
	configuration, err := config.Parse("config.yml", []byte(countsConfig))
	if err != nil {
		test.Fatal(err)
	}
	tabs := configuration.Tabs
	client := fake.New()
	number := 0
	for _, tab := range tabs {
		for _, section := range tab.Sections {
			rows := make([]github.PullRequest, 2)
			for i := range rows {
				number++
				rows[i] = pullRequest("acme/widgets", number, "title", time.Duration(number)*time.Hour)
			}
			client.SetSearchResults(searchQuery(section), rows)
		}
	}
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	return view
}

func pressCounted(view *View, sequence string, count int) {
	view.Update(keys.Message{Sequence: sequence, Count: count})
}

func TestCounts(test *testing.T) {
	tests := []struct {
		name  string
		start []string
		key   string
		count int
		want  int // selected row index
	}{
		{"3j moves three rows", nil, "j", 3, 3},
		{"j without a count moves once", nil, "j", 0, 1},
		{"a count larger than the list clamps", nil, "j", 9999, 7},
		{"a count larger than the list clamps up", []string{"G"}, "k", 9999, 0},
		{"3k moves up three", []string{"G"}, "k", 3, 4},
		{"5G goes to the fifth row", nil, "G", 5, 4},
		{"42G clamps to the last row", nil, "G", 42, 7},
		{"2] jumps two sections", nil, "]", 2, 4},
		{"9999] stops at the last section", nil, "]", 9999, 6},
		{"3[ jumps back three sections", []string{"G"}, "[", 3, 0},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := countsView(test)
			for _, key := range testCase.start {
				press(view, key)
			}
			pressCounted(view, testCase.key, testCase.count)
			if got := view.tabs[view.active].selectedIndex(); got != testCase.want {
				test.Errorf("selected row %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestCountOnTabKeysActsOnce(test *testing.T) {
	view := countsView(test)
	pressCounted(view, "g t", 3)
	if view.active != 1 {
		test.Errorf("3 g t: active tab = %d, want 1", view.active)
	}
	pressCounted(view, "g T", 3)
	if view.active != 0 {
		test.Errorf("3 g T: active tab = %d, want 0", view.active)
	}
}
