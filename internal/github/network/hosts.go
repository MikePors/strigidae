package network

import (
	"os"

	"github.com/cli/go-gh/v2/pkg/auth"

	"github.com/MikePors/strigidae/internal/github"
)

// KnownHost reports whether host is a GitHub host the client can talk to:
// one gh has credentials for, github.com, or the host named by GH_HOST.
// Hosts are compared with github.NormalizeHost.
func KnownHost(host string) bool {
	return knownHost(host, auth.KnownHosts(), os.Getenv("GH_HOST"))
}

// knownHost is KnownHost with gh's known hosts and GH_HOST injected, so tests
// never read the real gh config or environment.
func knownHost(host string, known []string, gitHubHost string) bool {
	host = github.NormalizeHost(host)
	if host == "" {
		return false
	}
	if host == "github.com" || host == github.NormalizeHost(gitHubHost) {
		return true
	}
	for _, k := range known {
		if github.NormalizeHost(k) == host {
			return true
		}
	}
	return false
}
