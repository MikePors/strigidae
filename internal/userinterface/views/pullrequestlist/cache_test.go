package pullrequestlist

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/listcache"
)

// Bubble Tea keeps its sequence message private; inspect only its command slice
// to deliver startup results separately, as the event loop would.
func startupCommands(test *testing.T, command tea.Cmd) (tea.Cmd, tea.Cmd) {
	test.Helper()
	sequence := reflect.ValueOf(command())
	if sequence.Kind() != reflect.Slice || sequence.Len() != 2 {
		test.Fatal("expected cache then live load")
	}
	return sequence.Index(0).Interface().(tea.Cmd), sequence.Index(1).Interface().(tea.Cmd)
}

func TestCachedStartup(test *testing.T) {
	for _, testCase := range []struct {
		name    string
		failure error
	}{
		{"fresh", nil}, {"offline", errors.New("offline")}, {"forbidden", github.ErrForbidden}, {"unauthenticated", github.ErrNotAuthenticated}, {"not_found", github.ErrNotFound},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			cache := listcache.New(test.TempDir(), "example.test", "alice", func() time.Time { return testNow })
			key := listcache.Key{Kind: "repository", Repository: testRepository, Limit: github.DefaultPageSize}
			if err := cache.Save(key, makePullRequests(2), testNow); err != nil {
				test.Fatal(err)
			}
			client := fakeWith(nil)
			client.SetError(fake.MethodListPullRequests, testCase.failure)
			view := newView(client, nil)
			view.cache = cache
			cached, fresh := startupCommands(test, view.Init())
			deliver(test, view, cached)
			if !view.Loading() || !view.current().stale || len(view.current().pullRequests) != 2 {
				test.Fatal("cached rows not shown while loading")
			}
			golden.Assert(test, "stale", ansi.Strip(view.Render(80, 13)), false)
			golden.Assert(test, "stale_narrow", ansi.Strip(view.Render(30, 8)), false)
			if next := view.maybeLoadMore(); next != nil {
				test.Fatal("paged stale results")
			}
			deliver(test, view, fresh)
			switch {
			case testCase.failure == nil:
				if view.current().stale || len(view.current().pullRequests) != 0 {
					test.Fatal("empty success did not replace stale list")
				}
				items, hit := cache.Load(key)
				if !hit || len(items) != 0 {
					test.Fatal("empty success not persisted")
				}
			case listcache.Denied(testCase.failure):
				if len(view.current().pullRequests) != 0 || view.current().stale {
					test.Fatal("access failure retained cached rows")
				}
				if _, hit := cache.Load(key); hit {
					test.Fatal("access failure retained cache")
				}
			default:
				if !view.current().stale || len(view.current().pullRequests) != 2 {
					test.Fatal("transient failure lost snapshot")
				}
			}
			// A delayed cached message cannot undo a completed fresh result.
			view.Update(cached())
			if view.Loading() {
				test.Fatal("late cache restarted loading")
			}
		})
	}
}

func TestCacheSupersededAndHostile(test *testing.T) {
	cache := listcache.New(test.TempDir(), "example.test", "alice", func() time.Time { return testNow })
	key := listcache.Key{Kind: "repository", Repository: testRepository, Limit: github.DefaultPageSize}
	items := makePullRequests(1)
	items[0].Title = "unsafe\x1b[2Jtitle"
	if err := cache.Save(key, items, testNow); err != nil {
		test.Fatal(err)
	}
	view := newView(fakeWith(nil), nil)
	view.cache = cache
	cached, _ := startupCommands(test, view.Init())
	message := cached()
	view.Update(message)
	if strings.Contains(view.Render(80, 13), "\x1b[2J") {
		test.Fatal("cached escape leaked")
	}
	refresh := view.reload(0)
	deliver(test, view, refresh)
	view.Update(message)
	if view.current().stale || len(view.current().pullRequests) != 0 {
		test.Fatal("superseded cache replaced refresh")
	}
}

func TestCacheMissStillLoads(test *testing.T) {
	view := newView(fakeWith(makePullRequests(2)), nil)
	view.cache = listcache.New(test.TempDir(), "example.test", "alice", func() time.Time { return testNow })
	cached, fresh := startupCommands(test, view.Init())
	deliver(test, view, cached)
	if view.current().stale || !view.Loading() {
		test.Fatal("miss changed loading state")
	}
	deliver(test, view, fresh)
	if view.Loading() || len(view.current().pullRequests) != 2 {
		test.Fatal("miss prevented live load")
	}
}

func TestCachedEmptyStartup(test *testing.T) {
	view := newView(fakeWith(makePullRequests(2)), nil)
	view.cache = listcache.New(test.TempDir(), "example.test", "alice", func() time.Time { return testNow })
	key := listcache.Key{Kind: "repository", Repository: testRepository, Limit: github.DefaultPageSize}
	if err := view.cache.Save(key, nil, testNow); err != nil {
		test.Fatal(err)
	}
	cached, fresh := startupCommands(test, view.Init())
	deliver(test, view, cached)
	rendered := ansi.Strip(view.Render(80, 13))
	if !strings.Contains(rendered, "stale") || !strings.Contains(rendered, "no open pull requests") {
		test.Fatal(rendered)
	}
	deliver(test, view, fresh)
	if view.current().stale || len(view.current().pullRequests) != 2 {
		test.Fatal("fresh rows did not replace empty snapshot")
	}
}

func TestDeniedNextPageRemovesSnapshot(test *testing.T) {
	client := fakeWith(makePullRequests(40))
	view := newView(client, nil)
	view.cache = listcache.New(test.TempDir(), "example.test", "alice", func() time.Time { return testNow })
	cached, fresh := startupCommands(test, view.Init())
	deliver(test, view, cached)
	deliver(test, view, fresh)
	key := listcache.Key{Kind: "repository", Repository: testRepository, Limit: github.DefaultPageSize}
	if _, hit := view.cache.Load(key); !hit {
		test.Fatal("first page was not cached")
	}
	client.SetError(fake.MethodListPullRequests, github.ErrForbidden)
	deliver(test, view, view.fetch(0, github.PageRequest{After: view.current().next}, true))
	if len(view.current().pullRequests) != 0 {
		test.Fatal("denied page retained rows")
	}
	if _, hit := view.cache.Load(key); hit {
		test.Fatal("denied page left a reusable snapshot")
	}
}
