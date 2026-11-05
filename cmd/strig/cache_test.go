package main

import (
	"testing"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
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
