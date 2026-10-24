package search

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMatch(test *testing.T) {
	rows := []string{
		"Cache the parsed diff octo-dev acme/widgets #12",
		"Fix a long title octo-dev acme/gadgets #7",
		"Tidy the shell prompt rivka rivka/dotfiles #86",
		"",
	}
	tests := []struct {
		name  string
		query string
		want  []int
	}{
		{"one word", "cache", []int{0}},
		{"case is ignored", "TIDY", []int{2}},
		{"every word must appear", "octo-dev acme", []int{0, 1}},
		{"words in any order", "diff cache", []int{0}},
		{"one word missing", "cache gadgets", nil},
		{"number with hash", "#86", []int{2}},
		{"number without hash", "86", []int{2}},
		{"owner and name", "acme/widgets", []int{0}},
		{"extra spaces", "  the   shell ", []int{2}},
		{"empty query matches nothing", "", nil},
		{"blank query matches nothing", "   ", nil},
		{"no match", "zzz", nil},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if diff := cmp.Diff(testCase.want, Match(testCase.query, rows)); diff != "" {
				test.Errorf("matches (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStateMovement(test *testing.T) {
	rows := []string{"a", "x1", "b", "x2", "c", "x3"}
	var state State
	state.Begin(2, "")
	row, ok := state.Type("x", rows, 2)
	if !ok || row != 3 {
		test.Fatalf("typing from row 2 went to %d, %v; want 3", row, ok)
	}
	if !state.Marked(1) || !state.Marked(5) || state.Marked(0) {
		test.Error("marks do not follow the matches")
	}
	state.Begin(4, "")
	if row, _ := state.Type("x", rows, 4); row != 5 {
		test.Errorf("from row 4 went to %d, want 5", row)
	}
	state.Begin(5, "")
	if row, _ := state.Type("x", rows, 5); row != 5 {
		test.Errorf("on a match stayed at %d, want 5", row)
	}
	state.Begin(4, "")
	if row, _ := state.Type("x1", rows, 4); row != 1 {
		test.Errorf("past the last match wrapped to %d, want 1", row)
	}
	state.Begin(2, "")
	state.Type("x", rows, 2)

	moves := []struct {
		name   string
		step   func(cursor, count int) (int, bool)
		cursor int
		count  int
		want   int
	}{
		{"next", state.Next, 1, 1, 3},
		{"next wraps", state.Next, 5, 1, 1},
		{"next with count", state.Next, 1, 3, 1},
		{"previous", state.Previous, 3, 1, 1},
		{"previous wraps", state.Previous, 1, 1, 5},
		{"previous with count", state.Previous, 5, 2, 1},
	}
	for _, move := range moves {
		if got, ok := move.step(move.cursor, move.count); !ok || got != move.want {
			test.Errorf("%s from %d x%d = %d, %v; want %d", move.name, move.cursor, move.count, got, ok, move.want)
		}
	}

	state.Refresh([]string{"x only"})
	if !state.Marked(0) || state.Marked(1) {
		test.Error("refresh did not recompute the marks")
	}
	state.Clear()
	if state.Active() || state.Marked(0) {
		test.Error("clear left a search behind")
	}
}

func TestCancelGoesBackToThePreviousSearch(test *testing.T) {
	rows := []string{"a", "b"}
	var state State
	state.Begin(0, "")
	state.Type("a", rows, 0)
	state.Begin(0, "")
	state.Type("b", rows, 0)
	state.Cancel(rows)
	if state.Query() != "a" || !state.Marked(0) || state.Marked(1) {
		test.Errorf("cancel left query %q", state.Query())
	}
}

func TestWhitespaceQueryIsNoSearch(test *testing.T) {
	var state State
	state.Begin(0, "")
	if _, ok := state.Type("   ", []string{"a b"}, 0); ok || state.Active() {
		test.Error("a blank query started a search")
	}
}
