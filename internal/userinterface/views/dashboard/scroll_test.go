package dashboard

import (
	"errors"
	"strings"
	"testing"

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

func TestFirstSectionEmptyKeepsItsHeadingAtTheTop(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	client.SetSearchResults(searchQuery(tabs[0].Sections[1]), []github.PullRequest{pullRequest("o/a", 1, "A1", 0), pullRequest("o/a", 2, "A2", 0)})
	client.SetSearchResults(searchQuery(tabs[0].Sections[2]), []github.PullRequest{pullRequest("o/b", 3, "B1", 0), pullRequest("o/b", 4, "B2", 0)})
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	press(view, "G")
	view.Render(80, 7)
	press(view, "g g")
	out := ansi.Strip(view.Render(80, 7))
	if !strings.Contains(out, "Review requested") || !strings.Contains(out, "No pull requests") {
		test.Errorf("the empty first section scrolled off:\n%s", out)
	}
}
