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
func normalizeQuery(q string) (string, error) {
	hasPR := false
	for _, tok := range strings.Fields(strings.ToLower(q)) {
		switch {
		case strings.HasPrefix(tok, "sort:"):
			return "", errors.New("query must not contain sort:; use the sort and order keys")
		case tok == "is:pr" || tok == "type:pr":
			hasPR = true
		}
	}
	if !hasPR {
		q = "is:pr " + q
	}
	for _, ph := range placeholder.FindAllString(q, -1) {
		if ph != "{user}" {
			return "", fmt.Errorf("unknown variable %s in query (the only variable is {user})", ph)
		}
	}
	return q, nil
}
