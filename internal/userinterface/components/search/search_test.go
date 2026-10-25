package search

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
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
		step   func(cursor, count int, wrap bool) (int, bool)
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
		if got, ok := move.step(move.cursor, move.count, true); !ok || got != move.want {
			test.Errorf("%s from %d x%d = %d, %v; want %d", move.name, move.cursor, move.count, got, ok, move.want)
		}
	}

	state.Refresh(List{Items: func() []Item { return []Item{{Text: "x only"}} }})
	if !state.Marked(0) || state.Marked(1) {
		test.Error("refresh did not recompute the marks")
	}
	state.Clear()
	if state.Active() || state.Marked(0) {
		test.Error("clear left a search behind")
	}
}

func TestNextAndPreviousWithoutWrapping(test *testing.T) {
	var state State
	state.Begin(0, "")
	state.Type("x", []string{"-", "x", "-", "x", "-", "x"}, 0) // matches at rows 1, 3 and 5
	moves := []struct {
		name   string
		step   func(cursor, count int, wrap bool) (int, bool)
		cursor int
		count  int
		want   int
		wantOK bool
	}{
		{"next", state.Next, 1, 1, 3, true},
		{"next at the last match", state.Next, 5, 1, 5, false},
		{"next below the last match", state.Next, 6, 1, 6, false},
		{"next with a count that runs out", state.Next, 3, 2, 3, false},
		{"previous", state.Previous, 3, 1, 1, true},
		{"previous at the first match", state.Previous, 1, 1, 1, false},
		{"previous above the first match", state.Previous, 0, 1, 0, false},
	}
	for _, move := range moves {
		got, ok := move.step(move.cursor, move.count, false)
		if got != move.want || ok != move.wantOK {
			test.Errorf("%s from %d x%d = %d, %v; want %d, %v", move.name, move.cursor, move.count, got, ok, move.want, move.wantOK)
		}
	}
}

func TestStepSaysWhereItStopped(test *testing.T) {
	cursor := 0
	list := List{
		Items:  func() []Item { return []Item{{Text: "x"}, {Text: "-"}, {Text: "x"}} },
		Cursor: func() int { return cursor },
		GoTo: func(row int) tea.Cmd {
			cursor = row
			return nil
		},
	}
	var state State
	state.Begin(0, "")
	state.Type("x", list.texts(), 0)

	tests := []struct {
		name     string
		forward  bool
		wrap     bool
		start    int
		wantRow  int
		wantText string
	}{
		{"n wraps", true, true, 2, 0, ""},
		{"N wraps", false, true, 0, 2, ""},
		{"n stops at the last match", true, false, 2, 2, "no more matches below"},
		{"N stops at the first match", false, false, 0, 0, "no more matches above"},
		{"n moves when it can", true, false, 0, 2, ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			cursor = testCase.start
			command := state.Step(keys.Message{}, testCase.forward, list, testCase.wrap)
			if cursor != testCase.wantRow {
				test.Errorf("cursor = %d, want %d", cursor, testCase.wantRow)
			}
			gotText := ""
			if command != nil {
				if info, ok := command().(userinterface.InfoMessage); ok {
					gotText = info.Text
				}
			}
			if gotText != testCase.wantText {
				test.Errorf("status = %q, want %q", gotText, testCase.wantText)
			}
		})
	}

	state.Clear()
	state.Begin(0, "")
	state.Type("zzz", list.texts(), 0)
	command := state.Step(keys.Message{}, true, list, false)
	if info, ok := command().(userinterface.InfoMessage); !ok || info.Text != "no match: zzz" {
		test.Errorf("with no match at all got %v", command())
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

func TestRefreshWithoutASearchDoesNothing(test *testing.T) {
	var state State
	state.Refresh(List{Items: func() []Item { return []Item{{Text: "a"}} }})
	if state.Active() || state.Marked(0) {
		test.Error("refresh made a search")
	}
}

func TestCursorKeyDefaultsToTheItemUnderTheCursor(test *testing.T) {
	items := []Item{{Text: "a", Identity: "ka"}, {Text: "b", Identity: "kb"}}
	list := List{Items: func() []Item { return items }}
	tests := []struct {
		name   string
		cursor int
		want   string
	}{{"first", 0, "ka"}, {"second", 1, "kb"}, {"past the end", 5, ""}, {"before the start", -1, ""}}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			list.Cursor = func() int { return testCase.cursor }
			if got := list.cursorKey(); got != testCase.want {
				test.Errorf("cursorKey = %q, want %q", got, testCase.want)
			}
		})
	}
	list.Cursor = func() int { return 0 }
	list.CursorKey = func() string { return "override" }
	if got := list.cursorKey(); got != "override" {
		test.Errorf("cursorKey = %q, want the override", got)
	}
}
