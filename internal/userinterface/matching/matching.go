// Package matching is the rule every "/" search uses to decide whether a row
// matches a query. It imports nothing from strig, so the shell's help overlay
// and the list search component share it.
package matching

import "strings"

// Rows returns the indices of the rows whose text matches query. A row matches
// when every space-separated word of the query appears in its text, ignoring
// case. An empty query matches nothing.
func Rows(query string, rows []string) []int {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	var matches []int
rows:
	for i, text := range rows {
		lower := strings.ToLower(text)
		for _, word := range words {
			if !strings.Contains(lower, word) {
				continue rows
			}
		}
		matches = append(matches, i)
	}
	return matches
}
