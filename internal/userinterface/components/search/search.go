// Package search is the bookkeeping for the "/" search in lists: the query,
// which rows match it, and where the cursor was when the search began. It
// knows nothing about rows beyond their text, so the dashboard, the pull
// request list and the Files pane share it.
//
// Currently every row stays and matching rows are marked; the matching rule
// is the single function Match, so another rule can replace it.
package search

import "strings"

// Glyph is drawn in the left column of a matching row, in the theme's
// SearchMarker style.
const Glyph = "•"

// Match returns the indices of the rows whose text matches query. Currently a
// row matches when every space-separated word of the query appears in its
// text, ignoring case. An empty query matches nothing.
func Match(query string, rows []string) []int {
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

// State is one list's search. The zero value is no search.
type State struct {
	query   string
	matches []int // ascending row indices
	marked  map[int]bool

	// While typing: where the cursor was, and the search to go back to.
	origin        int
	previousQuery string
}

// Begin starts typing a new search with the cursor at cursor.
func (state *State) Begin(cursor int) {
	state.origin, state.previousQuery = cursor, state.query
}

// Origin is the row the cursor was on when Begin was called.
func (state *State) Origin() int { return state.origin }

// Type sets the query being typed and returns the row the cursor should go
// to: the first match at or after the origin, wrapping to the first match.
// ok is false when nothing matches; the caller puts the cursor back on Origin.
func (state *State) Type(query string, rows []string) (row int, ok bool) {
	state.set(query, rows)
	return state.following(state.origin - 1)
}

// Cancel abandons the search being typed and goes back to the one before it.
func (state *State) Cancel(rows []string) { state.set(state.previousQuery, rows) }

// Clear drops the search and its marks.
func (state *State) Clear() { state.set("", nil) }

// Refresh recomputes the matches for rows that changed, keeping the query.
func (state *State) Refresh(rows []string) { state.set(state.query, rows) }

// Active reports whether there is a search, with or without matches.
func (state *State) Active() bool { return state.query != "" }

// Query is the current search text.
func (state *State) Query() string { return state.query }

// Marked reports whether row matches the search.
func (state *State) Marked(row int) bool { return state.marked[row] }

// Next returns the row count matches after cursor, wrapping. ok is false when
// nothing matches.
func (state *State) Next(cursor, count int) (row int, ok bool) {
	row = cursor
	for range max(count, 1) {
		if row, ok = state.following(row); !ok {
			return cursor, false
		}
	}
	return row, true
}

// Previous is Next going up.
func (state *State) Previous(cursor, count int) (row int, ok bool) {
	row = cursor
	for range max(count, 1) {
		if row, ok = state.preceding(row); !ok {
			return cursor, false
		}
	}
	return row, true
}

func (state *State) set(query string, rows []string) {
	state.query = query
	state.matches = Match(query, rows)
	state.marked = make(map[int]bool, len(state.matches))
	for _, row := range state.matches {
		state.marked[row] = true
	}
}

// following is the first match after row, or the first match overall.
func (state *State) following(row int) (int, bool) {
	for _, match := range state.matches {
		if match > row {
			return match, true
		}
	}
	if len(state.matches) == 0 {
		return 0, false
	}
	return state.matches[0], true
}

// preceding is the last match before row, or the last match overall.
func (state *State) preceding(row int) (int, bool) {
	for i := len(state.matches) - 1; i >= 0; i-- {
		if state.matches[i] < row {
			return state.matches[i], true
		}
	}
	if len(state.matches) == 0 {
		return 0, false
	}
	return state.matches[len(state.matches)-1], true
}
