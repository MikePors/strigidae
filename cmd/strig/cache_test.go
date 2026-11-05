package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/listcache"
)

type cacheIdentityClient struct {
	github.Client
	host    string
	account string
}

func (client cacheIdentityClient) Host() string         { return client.host }
func (client cacheIdentityClient) CacheAccount() string { return client.account }

func TestApplicationCacheIdentity(test *testing.T) {
	directory := test.TempDir()
	test.Setenv("XDG_CACHE_HOME", directory)
	test.Setenv("HOME", directory)
	test.Setenv("LocalAppData", directory)
	for _, testCase := range []struct {
		name    string
		client  github.Client
		enabled bool
	}{
		{"known account", cacheIdentityClient{Client: fake.New(), host: "example.test", account: "alice"}, true},
		{"unknown account", cacheIdentityClient{Client: fake.New(), host: "example.test"}, false},
		{"unknown host", cacheIdentityClient{Client: fake.New(), account: "alice"}, false},
		{"no identity support", fake.New(), false},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			harness := newHarness(nil, false, nil)
			harness.dependencies.newClient = func(string) (github.Client, error) { return testCase.client, nil }
			application, err := newApplication("acme/widgets", config.Default(), harness.dependencies)
			if err != nil {
				test.Fatal(err)
			}
			if enabled := application.cache != nil; enabled != testCase.enabled {
				test.Errorf("cache enabled=%v, want %v", enabled, testCase.enabled)
			}
		})
	}
}

func TestFirstViewShowsCachedRows(test *testing.T) {
	for _, testCase := range []struct{ name, repository string }{
		{"dashboard", ""}, {"repository", "acme/widgets"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			directory := test.TempDir()
			test.Setenv("XDG_CACHE_HOME", directory)
			test.Setenv("HOME", directory)
			test.Setenv("LocalAppData", directory)
			client := fake.New()
			harness := newHarness(nil, false, nil)
			harness.dependencies.newClient = func(string) (github.Client, error) {
				return cacheIdentityClient{Client: client, host: "example.test", account: "alice"}, nil
			}
			configuration := config.Default()
			application, err := newApplication(testCase.repository, configuration, harness.dependencies)
			if err != nil {
				test.Fatal(err)
			}
			if application.cache == nil {
				test.Fatal("known account has no cache")
			}
			key := listcache.Key{Kind: "repository", Repository: application.repository, Limit: github.DefaultPageSize}
			if application.dashboard {
				section := configuration.Tabs[0].Sections[0]
				key = listcache.Key{Kind: "search", Query: section.Query + " sort:" + string(section.Sort) + "-" + string(section.Order), Limit: section.Limit}
			}
			rows := []github.PullRequest{{Repository: github.Repository{Owner: "acme", Name: "widgets"}, Number: 42, Title: "Cached root row"}}
			if err := application.cache.Save(key, rows, time.Now()); err != nil {
				test.Fatal(err)
			}
			view := firstView(application)
			message := firstStartupMessage(test, view.Init())
			view.Update(message)
			rendered := ansi.Strip(view.Render(120, 20))
			if !strings.Contains(rendered, "Cached root row") || !strings.Contains(rendered, "(stale)") {
				test.Fatalf("root view did not show its cached row before refresh:\n%s", rendered)
			}
			if len(client.Calls()) != 0 {
				test.Fatal("startup needed a live request to show cached rows")
			}
		})
	}
}

// firstStartupMessage follows the first command in batches and sequences,
// stopping before the live refresh. Bubble Tea's sequence message is private.
func firstStartupMessage(test *testing.T, command tea.Cmd) tea.Msg {
	test.Helper()
	if command == nil {
		test.Fatal("expected startup command")
	}
	message := command()
	commands := reflect.ValueOf(message)
	if commands.Kind() == reflect.Slice && commands.Len() > 0 {
		return firstStartupMessage(test, commands.Index(0).Interface().(tea.Cmd))
	}
	return message
}
