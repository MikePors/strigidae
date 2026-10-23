package network

import "testing"

func TestKnownHost(t *testing.T) {
	known := []string{"github.com", "GHE.example.com"}
	tests := []struct {
		name   string
		host   string
		known  []string
		ghHost string
		want   bool
	}{
		{name: "github.com always", host: "github.com", want: true},
		{name: "github.com any case", host: "GitHub.com", want: true},
		{name: "ssh alias of github.com", host: "ssh.github.com", want: true},
		{name: "signed-in enterprise host", host: "ghe.example.com", known: known, want: true},
		{name: "signed-in host with port", host: "ghe.example.com:8443", known: known, want: true},
		{name: "GH_HOST", host: "ghe.corp.test", ghHost: "GHE.corp.test", want: true},
		{name: "other forge", host: "gitlab.com", known: known, want: false},
		{name: "ssh config alias", host: "github-work", known: known, ghHost: "ghe.corp.test", want: false},
		{name: "empty host with GH_HOST unset", host: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := knownHost(tt.host, tt.known, tt.ghHost); got != tt.want {
				t.Errorf("knownHost(%q, %q, %q) = %v, want %v", tt.host, tt.known, tt.ghHost, got, tt.want)
			}
		})
	}
}
