package network

import "testing"

func TestKnownHost(test *testing.T) {
	known := []string{"github.com", "GHE.example.com"}
	tests := []struct {
		name       string
		host       string
		known      []string
		gitHubHost string
		want       bool
	}{
		{name: "github.com always", host: "github.com", want: true},
		{name: "github.com any case", host: "GitHub.com", want: true},
		{name: "ssh alias of github.com", host: "ssh.github.com", want: true},
		{name: "signed-in enterprise host", host: "ghe.example.com", known: known, want: true},
		{name: "signed-in host with port", host: "ghe.example.com:8443", known: known, want: true},
		{name: "GH_HOST", host: "ghe.corp.test", gitHubHost: "GHE.corp.test", want: true},
		{name: "other forge", host: "gitlab.com", known: known, want: false},
		{name: "ssh config alias", host: "github-work", known: known, gitHubHost: "ghe.corp.test", want: false},
		{name: "empty host with GH_HOST unset", host: "", want: false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if got := knownHost(testCase.host, testCase.known, testCase.gitHubHost); got != testCase.want {
				test.Errorf("knownHost(%q, %q, %q) = %v, want %v", testCase.host, testCase.known, testCase.gitHubHost, got, testCase.want)
			}
		})
	}
}
