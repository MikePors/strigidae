package github

import (
	"fmt"
	"strings"
)

// Repo identifies a repository on the host the client is configured for.
type Repository struct {
	Owner string
	Name  string
}

// ParseRepo parses "owner/name".
func ParseRepository(s string) (Repository, error) {
	s = strings.TrimSpace(s)
	owner, name, ok := strings.Cut(s, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || strings.ContainsAny(s, " \t") {
		return Repository{}, fmt.Errorf("invalid repository %q: want owner/name", s)
	}
	return Repository{Owner: owner, Name: name}, nil
}

// String returns "owner/name".
func (r Repository) String() string { return r.Owner + "/" + r.Name }

// IsZero reports whether r is the zero Repo.
func (r Repository) IsZero() bool { return r == Repository{} }
