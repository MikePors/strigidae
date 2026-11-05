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
		{"fresh", nil}, {"offline", errors.New("offline")}, {"forbidden", github.ErrForbidden},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			cache := listcache.New(test.TempDir(), "example.test", "alice", nil)
			client := fake.New()
			client.SetError(fake.MethodSearchPullRequests, testCase.failure)
			view := newView(client, testTabs(test), nil)
			view.cache = cache
			target := &view.tabs[0].sections[0]
			key := listcache.Key{Kind: "search", Query: target.search, Limit: github.PageRequest{Limit: target.limit}.Size()}
			items := []github.PullRequest{pullRequest("acme/widgets", 12, "Cached pull request", time.Hour)}
			if err := cache.Save(key, items, time.Now()); err != nil {
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
