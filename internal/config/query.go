package config

import (
	"errors"
	"fmt"
	"strings"
)

// normalizeQuery validates a section's search query and returns it ready for
// use: a query that doesn't already select pull requests gets is:pr added. It
// rejects sort: qualifiers (the sort and order keys own sorting) and any {...}
// variable other than {user}.
func normalizeQuery(query string) (string, error) {
	hasPullRequest := false
	for _, token := range strings.Fields(strings.ToLower(query)) {
		switch {
		case strings.HasPrefix(token, "sort:"):
			return "", errors.New("query must not contain sort:; use the sort and order keys")
		case token == "is:pr" || token == "type:pr":
			hasPullRequest = true
		}
	}
	if !hasPullRequest {
		query = "is:pr " + query
	}
	for _, placeholderMatch := range placeholder.FindAllString(query, -1) {
		if placeholderMatch != "{user}" {
			return "", fmt.Errorf("unknown variable %s in query (the only variable is {user})", placeholderMatch)
		}
	}
	return query, nil
}
