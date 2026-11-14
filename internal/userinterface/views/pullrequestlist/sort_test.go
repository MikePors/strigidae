package pullrequestlist

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// sortBy runs `:sort arguments` and fails on anything but a quiet success.
func sortBy(test *testing.T, view *View, arguments string) {
	test.Helper()
	command, handled := view.Command("sort", arguments)
	if !handled {
		test.Fatal("sort not handled")
	}
	if command != nil {
		test.Fatalf(":sort %s: unexpected message %#v", arguments, command())
	}
}

func selectedNumber(view *View) int {
	pullRequest, _ := view.current().selected()
	return pullRequest.Number
}

func TestSortReordersAndKeepsTheSelection(test *testing.T) {
	view := loadedView(test, fakeWith(makePullRequests(5)), nil) // #5 down to #1
	press(view, "j")
	sortBy(test, view, "number")
	if diff := cmp.Diff([]int{1, 2, 3, 4, 5}, drawnNumbers(view)); diff != "" {
		test.Errorf("ascending (-want +got):\n%s", diff)
	}
	if got := selectedNumber(view); got != 4 {
		test.Errorf("cursor on #%d, want #4", got)
	}
	sortBy(test, view, "updated desc")
	if diff := cmp.Diff([]int{5, 4, 3, 2, 1}, drawnNumbers(view)); diff != "" {
		test.Errorf("updated desc (-want +got):\n%s", diff)
	}
	sortBy(test, view, "number")
	sortBy(test, view, "default")
	if diff := cmp.Diff([]int{5, 4, 3, 2, 1}, drawnNumbers(view)); diff != "" {
		test.Errorf("default (-want +got):\n%s", diff)
	}
	if got := selectedNumber(view); got != 4 {
		test.Errorf("after default the cursor is on #%d, want #4", got)
	}
}

func TestSortErrors(test *testing.T) {
	view := loadedView(test, fakeWith(makePullRequests(3)), nil)
	sortBy(test, view, "number")
	for _, testCase := range []struct{ arguments, want string }{
		{"", ":sort needs a key: number, title, author, created, updated, review, checks, comments, draft or default"},
		{"age", "unknown sort key: age"},
		{"title sideways", "usage: :sort <key> [asc|desc]"},
	} {
		failure, ok := commandMessage(test, view, "sort", testCase.arguments).(userinterface.ErrorMessage)
		if !ok || failure.Err.Error() != testCase.want {
			test.Errorf(":sort %q: message = %#v, want error %q", testCase.arguments, failure, testCase.want)
		}
	}
	// A refused :sort keeps the sort there was.
	if diff := cmp.Diff([]int{1, 2, 3}, drawnNumbers(view)); diff != "" {
		test.Errorf("after errors (-want +got):\n%s", diff)
	}
}

func TestSortSurvivesRefresh(test *testing.T) {
	client := fakeWith(makePullRequests(4))
	view := loadedView(test, client, nil)
	sortBy(test, view, "number")
	press(view, "g g")
	press(view, "j") // #2
	// #5 is new; the refresh asks for as many rows as were loaded, so #1 goes.
	client.SetPullRequests(testRepository, makePullRequests(5))
	deliver(test, view, press(view, "R"))
	if diff := cmp.Diff([]int{2, 3, 4, 5}, drawnNumbers(view)); diff != "" {
		test.Errorf("after refresh (-want +got):\n%s", diff)
	}
	if got := selectedNumber(view); got != 2 {
		test.Errorf("after refresh the cursor is on #%d, want #2", got)
	}
}

func TestSortTakesInANextPage(test *testing.T) {
	view := loadedView(test, fakeWith(makePullRequests(45)), nil) // page 1 holds #45 to #16
	sortBy(test, view, "number desc")
	if got := view.current().sortNote(); got != "↓ number (30 loaded)" {
		test.Errorf("note before the next page = %q", got)
	}
	command := press(view, "G") // #16, near the end: the next page is asked for
	deliver(test, view, command)
	numbers := drawnNumbers(view)
	if len(numbers) != 45 || numbers[0] != 45 || numbers[44] != 1 {
		test.Errorf("after the next page the rows run %v, want #45 down to #1", numbers)
	}
	if got := selectedNumber(view); got != 16 {
		test.Errorf("after the next page the cursor is on #%d, want #16", got)
	}
	if got := view.current().sortNote(); got != "↓ number" {
		test.Errorf("note with every page loaded = %q", got)
	}
}

func TestSortIsPerTab(test *testing.T) {
	client := fakeWith(makePullRequests(3))
	client.SetSearchResults("is:pr is:open repo:acme/widgets author:alice sort:updated-desc", authored("alice", 3, 1, 2))
	view := tabbedWith(test, client, config.Tab{Name: "Alice", User: "alice"})
	sortBy(test, view, "number")
	deliver(test, view, press(view, "l"))
	if diff := cmp.Diff([]int{3, 1, 2}, drawnNumbers(view)); diff != "" {
		test.Errorf("second tab (-want +got):\n%s", diff)
	}
	if got := view.current().sortNote(); got != "" {
		test.Errorf("second tab note = %q, want none", got)
	}
	press(view, "h")
	if diff := cmp.Diff([]int{1, 2, 3}, drawnNumbers(view)); diff != "" {
		test.Errorf("first tab (-want +got):\n%s", diff)
	}
}

func TestSortKeepsStacks(test *testing.T) {
	// Loaded 10, 3, 2, 1, 5; the stack is 1 <- 2 <- 3 with 5 also on 1.
	view := loadedView(test, fakeWith(stackedPullRequests()), nil)
	for _, testCase := range []struct {
		arguments string
		numbers   []int
		depths    []int
	}{
		{"number", []int{1, 2, 3, 5, 10}, []int{0, 1, 2, 1, 0}},
		{"number desc", []int{10, 1, 5, 2, 3}, []int{0, 0, 1, 1, 2}},
	} {
		sortBy(test, view, testCase.arguments)
		if diff := cmp.Diff(testCase.numbers, drawnNumbers(view)); diff != "" {
			test.Errorf(":sort %s rows (-want +got):\n%s", testCase.arguments, diff)
		}
		if diff := cmp.Diff(testCase.depths, view.current().depths); diff != "" {
			test.Errorf(":sort %s depths (-want +got):\n%s", testCase.arguments, diff)
		}
	}
}

func TestSortMovesTheSearchMarks(test *testing.T) {
	view := searchView(test) // "Fix" in #5, #3 and #1, drawn in rows 1, 3, 5
	typeSearch(view, "fix")
	sortBy(test, view, "title")
	// Add cache, Fix lexer, Fix parser, Fix prompt, Rename flag, Tidy docs.
	var marked []int
	for i := range view.current().pullRequests {
		if view.current().search.Marked(i) {
			marked = append(marked, i)
		}
	}
	if diff := cmp.Diff([]int{1, 2, 3}, marked); diff != "" {
		test.Errorf("marked rows (-want +got):\n%s", diff)
	}
}

func TestSortCompletion(test *testing.T) {
	view := loadedView(test, fakeWith(makePullRequests(1)), nil)
	want := []string{"number", "title", "author", "created", "updated", "review", "checks", "comments", "draft", "default"}
	if diff := cmp.Diff(want, view.CompleteArguments("sort")); diff != "" {
		test.Errorf("(-want +got):\n%s", diff)
	}
}

func TestSortGolden(test *testing.T) {
	for _, testCase := range []struct {
		name          string
		rows, width   int
		arguments     string
		height        int
		wantIndicator bool
	}{
		{"sorted", 4, 100, "created", 8, true},
		{"sorted_more_pages", 35, 100, "number desc", 8, true},
		{"sorted_narrow", 35, 36, "number desc", 8, false},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			view := loadedView(test, fakeWith(goldenSortRows(testCase.rows)), nil)
			sortBy(test, view, testCase.arguments)
			out := view.Render(testCase.width, testCase.height)
			for i, line := range strings.Split(out, "\n") {
				if width := ansi.StringWidth(line); width > testCase.width {
					test.Errorf("line %d is %d columns wide, max %d", i, width, testCase.width)
				}
			}
			tabBar := ansi.Strip(strings.Split(out, "\n")[0])
			if hasArrow := strings.ContainsAny(tabBar, "↑↓"); hasArrow != testCase.wantIndicator {
				test.Errorf("tab bar %q: indicator shown = %v, want %v", tabBar, hasArrow, testCase.wantIndicator)
			}
			golden.Assert(test, testCase.name, out, false)
		})
	}
}

// goldenSortRows are rows whose creation order differs from their number.
func goldenSortRows(count int) []github.PullRequest {
	rows := makePullRequests(count)
	for i := range rows {
		rows[i].CreatedAt = testNow.Add(-time.Duration((i*7)%count) * 24 * time.Hour)
	}
	return rows
}
