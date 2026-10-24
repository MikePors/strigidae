package pullrequestlist

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

func pressCounted(view *View, sequence string, count int) tea.Cmd {
	_, command := view.Update(keys.Message{Sequence: sequence, Count: count})
	return command
}

func TestCounts(test *testing.T) {
	tests := []struct {
		name  string
		start []string
		key   string
		count int
		want  int
	}{
		{"11j moves eleven rows", nil, "j", 11, 11},
		{"3k moves up three", []string{"G"}, "k", 3, 21},
		{"j without a count moves once", nil, "j", 0, 1},
		{"count larger than the list clamps down", nil, "j", 9999, 24},
		{"count larger than the list clamps up", []string{"G"}, "k", 9999, 0},
		{"42G clamps to the last row", nil, "G", 42, 24},
		{"7G goes to line seven", nil, "G", 7, 6},
		{"1G goes to the first line", []string{"j", "j"}, "G", 1, 0},
		{"G without a count goes to the bottom", nil, "G", 0, 24},
		{"2ctrl+d repeats the half page", nil, "ctrl+d", 2, 10},
		{"2ctrl+f repeats the page", nil, "ctrl+f", 2, 22},
		{"2ctrl+u repeats the half page up", []string{"G"}, "ctrl+u", 2, 14},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := loadedView(test, fakeWith(makePullRequests(25)), nil) // window shows 11 rows
			for _, key := range testCase.start {
				press(view, key)
			}
			pressCounted(view, testCase.key, testCase.count)
			if got := view.window.Cursor(); got != testCase.want {
				test.Errorf("cursor = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestCountOnRefreshActsOnce(test *testing.T) {
	view := loadedView(test, fakeWith(makePullRequests(25)), nil)
	press(view, "j")
	before := view.sequence
	if command := pressCounted(view, "R", 5); command == nil {
		test.Fatal("R with a count should still refresh")
	}
	if got := view.sequence - before; got != 1 {
		test.Errorf("5R started %d refreshes, want 1", got)
	}
	if view.window.Cursor() != 1 {
		test.Errorf("cursor = %d, want 1", view.window.Cursor())
	}
}

func TestCountOnOpenActsOnce(test *testing.T) {
	var opened []string
	view := loadedView(test, fakeWith(makePullRequests(3)), func(address string) error {
		opened = append(opened, address)
		return nil
	})
	command := pressCounted(view, "o", 5)
	if command == nil {
		test.Fatal("o should produce a command")
	}
	command()
	if len(opened) != 1 {
		test.Errorf("5o opened the browser %d times, want 1", len(opened))
	}
}
