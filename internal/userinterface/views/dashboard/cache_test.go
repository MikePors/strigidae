package dashboard

import (
	"errors"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/listcache"
)

func TestCachedStartup(test *testing.T) {
	for _, testCase := range []struct {
		name    string
		failure error
	}{
		{"fresh", nil}, {"offline", errors.New("offline")}, {"forbidden", github.ErrForbidden}, {"unauthenticated", github.ErrNotAuthenticated}, {"not_found", github.ErrNotFound},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			cache := listcache.New(test.TempDir(), "example.test", "alice", func() time.Time { return testNow })
			client := fake.New()
			client.SetError(fake.MethodSearchPullRequests, testCase.failure)
			view := newView(client, testTabs(test), nil)
			view.cache = cache
			target := &view.tabs[0].sections[0]
			key := listcache.Key{Kind: "search", Query: target.search, Limit: github.PageRequest{Limit: target.limit}.Size()}
			items := []github.PullRequest{pullRequest("acme/widgets", 12, "Cached pull request", time.Hour)}
			if err := cache.Save(key, items, testNow); err != nil {
				test.Fatal(err)
			}
			commands := view.Init()().(tea.BatchMsg)
			sequence := reflect.ValueOf(commands[0]())
			cached := sequence.Index(0).Interface().(tea.Cmd)
			fresh := sequence.Index(1).Interface().(tea.Cmd)
			view.Update(cached())
			if !target.stale || !target.loading || len(target.pullRequests) != 1 {
				test.Fatal("cached rows missing during refresh")
			}
			golden.Assert(test, "stale", ansi.Strip(view.Render(80, 16)), false)
			golden.Assert(test, "stale_narrow", ansi.Strip(view.Render(30, 10)), false)
			view.Update(fresh())
			if target.loading {
				test.Fatal("refresh still loading")
			}
			switch {
			case testCase.failure == nil:
				if target.stale || len(target.pullRequests) != 0 {
					test.Fatal("empty fresh result did not replace cached rows")
				}
			case listcache.Denied(testCase.failure):
				if _, hit := cache.Load(key); hit {
					test.Fatal("access failure retained cache")
				}
				if target.stale || len(target.pullRequests) != 0 {
					test.Fatal("access failure retained cached rows")
				}
			default:
				if !target.stale || len(target.pullRequests) != 1 {
					test.Fatal("offline failure lost cached rows")
				}
			}
			view.Update(cached())
			if target.loading {
				test.Fatal("late cache restarted loading")
			}
		})
	}
}

func TestDeniedSectionKeepsFallbackSelection(test *testing.T) {
	view := newView(fake.New(), testTabs(test), nil)
	current := &view.tabs[0]
	first := pullRequest("acme/widgets", 1, "First", time.Hour)
	second := pullRequest("acme/widgets", 2, "Second", time.Hour)
	current.sections[0].pullRequests = []github.PullRequest{first}
	current.sections[1].pullRequests = []github.PullRequest{second}
	current.selectIndex(0)
	view.loaded(sectionLoadedMessage{tab: 0, section: 0, err: github.ErrForbidden})
	selected, ok := current.selectedPullRequest()
	if !ok || selected.Number != second.Number {
		test.Fatal("cursor did not fall back")
	}
	view.loaded(sectionLoadedMessage{tab: 0, section: 0, page: github.Page[github.PullRequest]{Items: []github.PullRequest{first}}})
	selected, ok = current.selectedPullRequest()
	if !ok || selected.Number != second.Number {
		test.Fatal("cursor jumped back to the recovered section")
	}
}

func TestRefreshSkipsCachedSnapshot(test *testing.T) {
	client := fake.New()
	view := newView(client, testTabs(test), nil)
	target := &view.tabs[0].sections[0]
	original := pullRequest("acme/widgets", 1, "Original live result", time.Hour)
	client.SetSearchResults(target.search, []github.PullRequest{original})
	settle(view, view.Init())
	view.cache = listcache.New(test.TempDir(), "example.test", "alice", func() time.Time { return testNow })
	key := listcache.Key{Kind: "search", Query: target.search, Limit: github.PageRequest{Limit: target.limit}.Size()}
	stale := pullRequest("acme/widgets", 2, "Old snapshot", time.Hour)
	if err := view.cache.Save(key, []github.PullRequest{stale}, testNow); err != nil {
		test.Fatal(err)
	}
	fresh := pullRequest("acme/widgets", 3, "Fresh result", time.Hour)
	client.SetSearchResults(target.search, []github.PullRequest{fresh})
	commands := press(view, "R")().(tea.BatchMsg)
	if target.stale || len(target.pullRequests) != 1 || target.pullRequests[0].Number != original.Number {
		test.Fatal("refresh changed the current rows before loading")
	}
	message, ok := commands[0]().(sectionLoadedMessage)
	if !ok || message.cached {
		test.Fatal("refresh read the disk snapshot instead of loading live results")
	}
	view.Update(message)
	if target.stale || target.loading || len(target.pullRequests) != 1 || target.pullRequests[0].Number != fresh.Number {
		test.Fatal("refresh did not install fresh results")
	}
}
