package pullrequestlist

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func authored(author string, numbers ...int) []github.PullRequest {
	var pullRequests []github.PullRequest
	for i, number := range numbers {
		pullRequests = append(pullRequests, github.PullRequest{
			Repository: testRepository,
			Number:     number,
			Title:      "By " + author,
			Author:     author,
			State:      github.PullRequestStateOpen,
			URL:        "https://example.test/" + author,
			UpdatedAt:  testNow.Add(-time.Duration(i) * time.Hour),
		})
	}
	return pullRequests
}

func tabbedView(configured ...config.Tab) *View {
	view := New(fake.New(), testRepository, configured, theme.Default(), Options{Now: func() time.Time { return testNow }})
	view.Update(tea.WindowSizeMsg{Width: 80, Height: 13})
	return view
}

func tabbedWith(test *testing.T, client github.Client, configured ...config.Tab) *View {
	test.Helper()
	view := New(client, testRepository, configured, theme.Default(), Options{Now: func() time.Time { return testNow }})
	view.Update(tea.WindowSizeMsg{Width: 80, Height: 13})
	deliver(test, view, view.Init())
	return view
}

func tabNames(view *View) []string {
	var names []string
	for _, current := range view.tabs {
		names = append(names, current.name)
	}
	return names
}

func TestTabsFromConfig(test *testing.T) {
	for _, testCase := range []struct {
		name       string
		configured []config.Tab
		wantNames  []string
		wantQuery  []string
	}{
		{"no tabs", nil, []string{"All"}, []string{""}},
		{"default config", config.Default().Tabs, []string{"All", "Me"}, []string{"", "is:pr is:open repo:acme/widgets author:@me sort:updated-desc"}},
		{
			"configured order and users",
			[]config.Tab{{Name: "Alice", User: "alice"}, {Name: "Bob", User: "bob-b"}, {Name: "No user"}},
			[]string{"All", "Alice", "Bob", "No user"},
			[]string{
				"",
				"is:pr is:open repo:acme/widgets author:alice sort:updated-desc",
				"is:pr is:open repo:acme/widgets author:bob-b sort:updated-desc",
				"is:pr is:open repo:acme/widgets author:@me sort:updated-desc",
			},
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			view := tabbedView(testCase.configured...)
			if diff := cmp.Diff(testCase.wantNames, tabNames(view)); diff != "" {
				test.Errorf("names (-want +got):\n%s", diff)
			}
			var queries []string
			for _, current := range view.tabs {
				queries = append(queries, current.query)
			}
			if diff := cmp.Diff(testCase.wantQuery, queries); diff != "" {
				test.Errorf("queries (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSwitchingTabsWraps(test *testing.T) {
	view := tabbedView(config.Tab{Name: "Alice", User: "alice"}, config.Tab{Name: "Bob", User: "bob"})
	for _, step := range []struct {
		key  string
		want int
	}{{"g t", 1}, {"g t", 2}, {"g t", 0}, {"g T", 2}, {"g T", 1}} {
		press(view, step.key)
		if view.active != step.want {
			test.Fatalf("after %q: active = %d, want %d", step.key, view.active, step.want)
		}
	}
}

func TestTabsLoadWhenFirstShown(test *testing.T) {
	client := fakeWith(makePullRequests(2))
	client.SetSearchResults("is:pr is:open repo:acme/widgets author:alice sort:updated-desc", authored("alice", 21, 22, 23))
	client.SetSearchResults("is:pr is:open repo:acme/widgets author:bob sort:updated-desc", authored("bob", 31))
	view := tabbedWith(test, client, config.Tab{Name: "Alice", User: "alice"}, config.Tab{Name: "Bob", User: "bob"})

	wantCalls := []fake.Call{{Method: fake.MethodListPullRequests, Repository: testRepository}}
	if diff := cmp.Diff(wantCalls, client.Calls()); diff != "" {
		test.Fatalf("only the first tab loads at start (-want +got):\n%s", diff)
	}

	command := press(view, "g t")
	if !view.Loading() {
		test.Error("a tab being loaded for the first time shows as loading")
	}
	deliver(test, view, command)
	if len(view.current().pullRequests) != 3 {
		test.Fatalf("alice rows = %d, want 3", len(view.current().pullRequests))
	}
	wantCalls = append(wantCalls, fake.Call{Method: fake.MethodSearchPullRequests, Query: "is:pr is:open repo:acme/widgets author:alice sort:updated-desc"})
	if diff := cmp.Diff(wantCalls, client.Calls()); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}

	// Back on All, then on Alice again: shown tabs are not reloaded.
	if command := press(view, "g T"); command != nil {
		test.Error("switching to a loaded tab must not load")
	}
	if command := press(view, "g t"); command != nil {
		test.Error("switching to a loaded tab must not load")
	}

	// R refreshes only the shown tab.
	deliver(test, view, press(view, "R"))
	wantCalls = append(wantCalls, fake.Call{Method: fake.MethodSearchPullRequests, Query: "is:pr is:open repo:acme/widgets author:alice sort:updated-desc", Page: github.PageRequest{Limit: 3}})
	if diff := cmp.Diff(wantCalls, client.Calls()); diff != "" {
		test.Errorf("calls after R (-want +got):\n%s", diff)
	}
	if len(view.tabs[2].pullRequests) != 0 || view.tabs[2].requested {
		test.Error("the Bob tab was touched")
	}
}

func TestEachTabKeepsItsCursorAndSearch(test *testing.T) {
	client := fakeWith(makePullRequests(5))
	client.SetSearchResults("is:pr is:open repo:acme/widgets author:alice sort:updated-desc", authored("alice", 21, 22, 23, 24))
	view := tabbedWith(test, client, config.Tab{Name: "Alice", User: "alice"})

	press(view, "j")
	press(view, "j")
	press(view, "/")
	typeSearch(view, "Change number 1")
	finishSearch(view, "Change number 1", userinterface.PromptSubmitted)
	allCursor := view.current().window.Cursor()
	allQuery := view.current().search.Query()

	deliver(test, view, press(view, "g t"))
	if view.current().window.Cursor() != 0 || view.current().search.Query() != "" {
		test.Errorf("alice tab: cursor %d, search %q; want a fresh tab", view.current().window.Cursor(), view.current().search.Query())
	}
	press(view, "j")
	press(view, "j")
	press(view, "j")
	if view.current().window.Cursor() != 3 {
		test.Fatalf("alice cursor = %d, want 3", view.current().window.Cursor())
	}

	press(view, "g T")
	if view.current().window.Cursor() != allCursor || view.current().search.Query() != allQuery {
		test.Errorf("All tab lost its state: cursor %d, search %q", view.current().window.Cursor(), view.current().search.Query())
	}
	press(view, "g t")
	if view.current().window.Cursor() != 3 {
		test.Errorf("alice cursor = %d after returning, want 3", view.current().window.Cursor())
	}
}

func TestEnterAndOpenUseTheShownTab(test *testing.T) {
	client := fakeWith(makePullRequests(2))
	client.SetSearchResults("is:pr is:open repo:acme/widgets author:alice sort:updated-desc", authored("alice", 21, 22))
	var opened []string
	view := tabbedWith(test, client, config.Tab{Name: "Alice", User: "alice"})
	view.open = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	deliver(test, view, press(view, "g t"))
	press(view, "j")
	if command := press(view, "enter"); command == nil {
		test.Error("enter on an author tab opens the pull request")
	}
	command := press(view, "o")
	if command == nil {
		test.Fatal("o should open the browser")
	}
	command()
	if diff := cmp.Diff([]string{"https://example.test/alice"}, opened); diff != "" {
		test.Errorf("opened (-want +got):\n%s", diff)
	}
}

func TestTabStates(test *testing.T) {
	configured := config.Tab{Name: "Alice", User: "alice"}

	test.Run("loading", func(test *testing.T) {
		view := tabbedWith(test, fakeWith(makePullRequests(2)), configured)
		press(view, "g t")
		if out := ansi.Strip(view.Render(80, 13)); !strings.Contains(out, "Loading pull requests in acme/widgets...") {
			test.Errorf("loading text missing:\n%s", out)
		}
	})

	test.Run("empty", func(test *testing.T) {
		view := tabbedWith(test, fakeWith(makePullRequests(2)), configured)
		deliver(test, view, press(view, "g t"))
		if out := ansi.Strip(view.Render(80, 13)); !strings.Contains(out, "no open pull requests by alice in acme/widgets") {
			test.Errorf("empty text missing:\n%s", out)
		}
	})

	test.Run("error stays on its tab", func(test *testing.T) {
		client := fakeWith(makePullRequests(2))
		client.SetError(fake.MethodSearchPullRequests, errors.New("search broke"))
		view := tabbedWith(test, client, configured)
		deliver(test, view, press(view, "g t"))
		out := ansi.Strip(view.Render(80, 13))
		if !strings.Contains(out, "Could not load pull requests") || !strings.Contains(out, "search broke") {
			test.Errorf("error text missing:\n%s", out)
		}
		press(view, "g T")
		if out := ansi.Strip(view.Render(80, 13)); strings.Contains(out, "search broke") {
			test.Errorf("the All tab shows the other tab's error:\n%s", out)
		}
	})

	test.Run("late result lands in its own tab", func(test *testing.T) {
		client := fakeWith(makePullRequests(2))
		client.SetSearchResults("is:pr is:open repo:acme/widgets author:alice sort:updated-desc", authored("alice", 21))
		view := tabbedWith(test, client, configured)
		command := press(view, "g t")
		press(view, "g T") // leave before the answer arrives
		deliver(test, view, command)
		if len(view.tabs[0].pullRequests) != 2 || len(view.tabs[1].pullRequests) != 1 {
			test.Errorf("rows All=%d Alice=%d, want 2 and 1", len(view.tabs[0].pullRequests), len(view.tabs[1].pullRequests))
		}
	})
}

func TestHostileTabNameAndUser(test *testing.T) {
	view := tabbedView(config.Tab{Name: "Al\x1b]52;c;aGk=\x07ice", User: "al\x1b[2Jice"})
	// Colour codes are the theme's own; any other escape or BEL is hostile.
	raw := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(view.Render(80, 13), "")
	if strings.ContainsAny(raw, "\x1b\x07") {
		test.Errorf("control characters reached the screen: %q", raw)
	}
	if strings.ContainsAny(view.tabs[1].query, "\x1b\x07") {
		test.Errorf("query carries control characters: %q", view.tabs[1].query)
	}
}

func TestSearchMarksFollowRowsThatArriveInAHiddenTab(test *testing.T) {
	client := fakeWith(makePullRequests(5)) // #5 ... #1
	view := tabbedWith(test, client, config.Tab{Name: "Alice", User: "alice"})
	press(view, "/")
	typeSearch(view, "number 2")
	finishSearch(view, "number 2", userinterface.PromptSubmitted)
	if !view.current().search.Marked(3) {
		test.Fatal("setup: #2 should be marked on row 3")
	}

	refresh := press(view, "R")
	press(view, "g t") // leave before the answer arrives
	reversed := makePullRequests(5)
	slices.Reverse(reversed)
	for i := range reversed {
		reversed[i].UpdatedAt = testNow.Add(-time.Duration(i) * time.Hour) // the fake lists newest first
	}
	client.SetPullRequests(testRepository, reversed) // #1 ... #5, newest first
	deliver(test, view, refresh)
	press(view, "g T")

	if !view.current().search.Marked(1) || view.current().search.Marked(3) {
		test.Errorf("marks: row 1 %v, row 3 %v; want #2 (now row 1) marked and row 3 not", view.current().search.Marked(1), view.current().search.Marked(3))
	}
}

func TestHiddenTabErrorNamesTheTab(test *testing.T) {
	client := fakeWith(makePullRequests(2))
	client.SetError(fake.MethodSearchPullRequests, errors.New("search broke"))
	view := tabbedWith(test, client, config.Tab{Name: "Al\x1b[2Jice", User: "alice"})
	command := press(view, "g t")
	press(view, "g T")
	_, next := view.Update(command())
	status := next().(userinterface.ErrorMessage).Err.Error()
	if want := "Al [2Jice: load pull requests in acme/widgets: search broke"; status != want {
		test.Errorf("status = %q, want %q", status, want)
	}

	// The shown tab's own error does not repeat its name.
	press(view, "g t")
	_, next = view.Update(pullRequestsLoadedMessage{repository: testRepository, tab: 1, sequence: view.tabs[1].sequence, err: errors.New("again")})
	if status := next().(userinterface.ErrorMessage).Err.Error(); strings.HasPrefix(status, "Al") {
		test.Errorf("status = %q, want no tab name on the shown tab", status)
	}
}

func TestTabGoldens(test *testing.T) {
	configured := []config.Tab{{Name: "Alice", User: "alice"}, {Name: "Bob", User: "bob"}}
	test.Run("second_tab", func(test *testing.T) {
		client := fakeWith(makePullRequests(3))
		client.SetSearchResults("is:pr is:open repo:acme/widgets author:alice sort:updated-desc", authored("alice", 21, 22))
		view := tabbedWith(test, client, configured...)
		deliver(test, view, press(view, "g t"))
		golden.Assert(test, "second_tab", view.Render(60, 8), false)
	})
	test.Run("tab_empty", func(test *testing.T) {
		view := tabbedWith(test, fakeWith(makePullRequests(3)), configured...)
		deliver(test, view, press(view, "g t"))
		golden.Assert(test, "tab_empty", view.Render(60, 6), false)
	})
	test.Run("tab_error", func(test *testing.T) {
		client := fakeWith(makePullRequests(3))
		client.SetError(fake.MethodSearchPullRequests, errors.New("search broke"))
		view := tabbedWith(test, client, configured...)
		deliver(test, view, press(view, "g t"))
		golden.Assert(test, "tab_error", view.Render(60, 8), false)
	})
	test.Run("tab_loading", func(test *testing.T) {
		view := tabbedWith(test, fakeWith(makePullRequests(3)), configured...)
		press(view, "g t")
		golden.Assert(test, "tab_loading", view.Render(60, 6), false)
	})
}
