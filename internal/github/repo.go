package github

import (
	"fmt"
	"strings"
)

// Repository identifies a repository on the host the client is configured for.
type Repository struct {
	Owner string
	Name  string
}

// ParseRepository parses "owner/name".
func ParseRepository(text string) (Repository, error) {
	text = strings.TrimSpace(text)
	owner, name, ok := strings.Cut(text, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || strings.ContainsAny(text, " \t") {
		return Repository{}, fmt.Errorf("invalid repository %q: want owner/name", text)
	}
	return Repository{Owner: owner, Name: name}, nil
}

// String returns "owner/name".
func (repository Repository) String() string { return repository.Owner + "/" + repository.Name }

// IsZero reports whether r is the zero Repository.
func (repository Repository) IsZero() bool { return repository == Repository{} }
