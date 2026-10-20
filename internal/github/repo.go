package github

import (
	"fmt"
	"strings"
)

// Repo identifies a repository on the host the client is configured for.
type Repo struct {
	Owner string
	Name  string
}

// ParseRepo parses "owner/name".
func ParseRepo(s string) (Repo, error) {
	s = strings.TrimSpace(s)
	owner, name, ok := strings.Cut(s, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || strings.ContainsAny(s, " \t") {
		return Repo{}, fmt.Errorf("invalid repository %q: want owner/name", s)
	}
	return Repo{Owner: owner, Name: name}, nil
}

// String returns "owner/name".
func (r Repo) String() string { return r.Owner + "/" + r.Name }

// IsZero reports whether r is the zero Repo.
func (r Repo) IsZero() bool { return r == Repo{} }
