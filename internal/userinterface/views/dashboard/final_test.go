package dashboard

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestTrailingSectionsAreVisibleAtTheEnd(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	var rows []github.PullRequest
	for i := range 10 {
		rows = append(rows, pullRequest("acme/widgets", i+1, "Row", 0))
	}
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), rows)
	view := newView(client, tabs, nil)
	all := messages(view.Init())
	failed := all[2].(sectionLoadedMessage) // Involved fails; My PRs is empty
	failed.err = errors.New("boom")
	deliver(view, []tea.Msg{all[0], all[1], failed})

	for _, sequence := range []string{"G", "gg-then-j"} {
		if sequence == "G" {
			press(view, "G")
		} else {
			press(view, "g g")
			for range 9 {
				press(view, "j")
				view.Render(80, 10)
			}
		}
		out := ansi.Strip(view.Render(80, 10))
		for _, want := range []string{"No pull requests", "Could not load: boom", "Involved"} {
			if !strings.Contains(out, want) {
				test.Errorf("after %s the screen lacks %q:\n%s", sequence, want, out)
			}
		}
	}
}

func TestHeadingStaysVisibleWhenJumpingBackASection(test *testing.T) {
	configured := func(name string) config.Section {
		return config.Section{Name: name, Query: "is:pr " + name, Limit: 5, Sort: config.SortUpdated, Order: config.OrderDescending}
	}
	tabs := []config.Tab{{Name: "T", Sections: []config.Section{configured("Alpha"), configured("Beta"), configured("Gamma")}}}
	client := fake.New()
	for i, section := range tabs[0].Sections {
		var rows []github.PullRequest
		for row := range 3 {
			rows = append(rows, pullRequest("o/r", i*10+row+1, "Row", 0))
		}
		client.SetSearchResults(searchQuery(section), rows)
	}
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	press(view, "G")
	view.Render(80, 5)
	press(view, "[") // first row of Beta
	if got := selectedKey(test, view); got != "o/r#11" {
		test.Fatalf("[ selected %s, want o/r#11", got)
	}
	if out := ansi.Strip(view.Render(80, 5)); !strings.Contains(out, "Beta") {
		test.Errorf("Beta's heading is off screen:\n%s", out)
	}
}

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
