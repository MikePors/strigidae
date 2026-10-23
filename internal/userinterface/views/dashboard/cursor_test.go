package dashboard

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestPosition(test *testing.T) {
	rows := func(numbers ...int) section {
		var found []github.PullRequest
		for _, number := range numbers {
			found = append(found, pullRequest("o/r", number, "T", 0))
		}
		return section{pullRequests: found}
	}
	anchored := func(sectionIndex, number, row int) *anchor {
		return &anchor{section: sectionIndex, repository: github.Repository{Owner: "o", Name: "r"}, number: number, row: row}
	}
	tests := []struct {
		name        string
		sections    []section
		anchor      *anchor
		wantSection int
		wantRow     int
		wantOK      bool
	}{
		{"no anchor starts at the first non-empty section", []section{rows(), rows(5, 6)}, nil, 1, 0, true},
		{"found by identity", []section{rows(1, 2, 3)}, anchored(0, 2, 0), 0, 1, true},
		{"identity is matched only within the section", []section{rows(7), rows(1, 7)}, anchored(1, 7, 0), 1, 1, true},
		{"pull request gone: the fallback row", []section{rows(1, 2, 3, 4)}, anchored(0, 99, 2), 0, 2, true},
		{"fallback row past the end clamps", []section{rows(1, 2)}, anchored(0, 99, 5), 0, 1, true},
		{"emptied section: nearest below first", []section{rows(), rows(5), rows(6)}, anchored(0, 1, 0), 1, 0, true},
		{"emptied last section: nearest above", []section{rows(1, 2), rows()}, anchored(1, 9, 0), 0, 1, true},
		{"nothing at all", []section{rows(), rows()}, anchored(0, 1, 0), 0, 0, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			current := &tab{sections: slices.Clone(testCase.sections), anchor: testCase.anchor}
			section, row, ok := current.position()
			if section != testCase.wantSection || row != testCase.wantRow || ok != testCase.wantOK {
				test.Errorf("position = (%d, %d, %v), want (%d, %d, %v)", section, row, ok, testCase.wantSection, testCase.wantRow, testCase.wantOK)
			}
		})
	}
}

func TestRefillAfterEveryRowDisappearsStartsAtTheTop(test *testing.T) {
	for _, reverse := range []bool{false, true} {
		test.Run(fmt.Sprintf("reverse=%v", reverse), func(test *testing.T) {
			tabs := testTabs(test)
			client := fake.New()
			seed(test, client, tabs)
			view := newView(client, tabs, nil)
			settle(view, view.Init())
			press(view, "G") // away from the top

			for _, section := range tabs[0].Sections {
				client.SetSearchResults(searchQuery(section), nil)
			}
			settle(view, press(view, "R")) // every section is empty now

			client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{pullRequest("acme/widgets", 12, "First", time.Hour)})
			client.SetSearchResults(searchQuery(tabs[0].Sections[2]), []github.PullRequest{pullRequest("acme/widgets", 230, "Last", time.Hour)})
			all := messages(press(view, "R"))
			if reverse {
				slices.Reverse(all)
			}
			deliver(view, all)
			if got := selectedKey(test, view); got != "acme/widgets#12" {
				test.Errorf("selected %s, want the first row of the first non-empty section", got)
			}
		})
	}
}

func TestCursorStaysWhereItIsShownWhenItsSectionEmptiesAndRefills(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	press(view, "g g") // anchor on the first row of section 0
	first := searchQuery(tabs[0].Sections[0])
	refill := []github.PullRequest{pullRequest("acme/widgets", 12, "Cache the parsed diff", time.Hour)}

	client.SetSearchResults(first, nil)
	settle(view, press(view, "R"))
	shown := selectedKey(test, view)
	if shown != "rivka/dotfiles#3" {
		test.Fatalf("with section 0 empty the cursor shows %s, want rivka/dotfiles#3", shown)
	}
	client.SetSearchResults(first, refill)
	settle(view, press(view, "R"))
	if got := selectedKey(test, view); got != shown {
		test.Errorf("after section 0 refilled the cursor jumped to %s, want it to stay on %s", got, shown)
	}
}
