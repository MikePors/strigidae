package matching

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRows(test *testing.T) {
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
		{"query case differs from the row", "tIdY", []int{2}},
		{"row case differs from the query", "CACHE", []int{0}},
		{"every word must appear", "octo-dev acme", []int{0, 1}},
		{"words in any order", "diff cache", []int{0}},
		{"one word missing", "cache gadgets", nil},
		{"extra spaces", "  the   shell ", []int{2}},
		{"empty query matches nothing", "", nil},
		{"blank query matches nothing", "   ", nil},
		{"no match", "zzz", nil},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if diff := cmp.Diff(testCase.want, Rows(testCase.query, rows)); diff != "" {
				test.Errorf("matches (-want +got):\n%s", diff)
			}
		})
	}
}
