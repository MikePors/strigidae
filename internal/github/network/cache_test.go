package network

import (
	"testing"

	ghconfig "github.com/cli/go-gh/v2/pkg/config"
)

func TestCacheAccount(test *testing.T) {
	configuration := ghconfig.ReadFromString("hosts:\n  example.test:\n    user: alice\n")
	for _, testCase := range []struct{ source, host, want string }{
		{"oauth_token", "example.test", "alice"},
		{"gh", "example.test", "alice"},
		{"GH_TOKEN", "example.test", ""},
		{"GITHUB_TOKEN", "example.test", ""},
		{"GH_ENTERPRISE_TOKEN", "example.test", ""},
		{"GITHUB_ENTERPRISE_TOKEN", "example.test", ""},
		{"default", "example.test", ""},
		{"gh", "unknown.test", ""},
	} {
		test.Run(testCase.source+testCase.host, func(test *testing.T) {
			if got := cacheAccount(configuration, testCase.host, testCase.source); got != testCase.want {
				test.Errorf("account=%q, want %q", got, testCase.want)
			}
		})
	}
}
