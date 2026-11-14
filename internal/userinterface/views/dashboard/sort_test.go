package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/pullrequestmarks"
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

// sectionRows lists each shown section's rows as owner/name#number.
func sectionRows(view *View) [][]string {
	var out [][]string
	for _, target := range view.tabs[view.active].displaySections() {
		rows := []string{}
		for _, pullRequest := range target.pullRequests {
			rows = append(rows, fmt.Sprintf("%s#%d", pullRequest.Repository, pullRequest.Number))
		}
		out = append(out, rows)
	}
	return out
}

func TestSortReordersEachSectionAndKeepsTheSelection(test *testing.T) {
	view, client := loadedView(test)
	before := searches(client)
	press(view, "j") // acme/gadgets#7, the second row of the first section
	sortBy(test, view, "number")
	want := [][]string{{"acme/gadgets#7", "acme/widgets#12"}, {"rivka/dotfiles#3"}, {}}
	if diff := cmp.Diff(want, sectionRows(view)); diff != "" {
		test.Errorf("sorted (-want +got):\n%s", diff)
	}
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("cursor on %s, want acme/gadgets#7", got)
	}
	if searches(client) != before {
		test.Error(":sort searched GitHub")
	}
	press(view, "j")
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("j moved to %s, want acme/widgets#12, the next row in the sorted order", got)
	}

	sortBy(test, view, "default")
	want = [][]string{{"acme/widgets#12", "acme/gadgets#7"}, {"rivka/dotfiles#3"}, {}}
	if diff := cmp.Diff(want, sectionRows(view)); diff != "" {
		test.Errorf("default (-want +got):\n%s", diff)
	}
}

func TestSortSurvivesRefresh(test *testing.T) {
	view, _ := loadedView(test)
	sortBy(test, view, "number")
	press(view, "g g") // acme/gadgets#7
	settle(view, press(view, "R"))
	if diff := cmp.Diff([]string{"acme/gadgets#7", "acme/widgets#12"}, sectionRows(view)[0]); diff != "" {
		test.Errorf("after refresh (-want +got):\n%s", diff)
	}
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("after refresh the cursor is on %s, want acme/gadgets#7", got)
	}
}

func TestSortIsPerTab(test *testing.T) {
	view, _ := loadedView(test)
	sortBy(test, view, "number")
	settle(view, press(view, "g t"))
	if label := view.tabs[view.active].order.Label(); label != "" {
		test.Errorf("second tab sort = %q, want none", label)
	}
	press(view, "g T")
	if label := view.tabs[view.active].order.Label(); label != "↑ number" {
		test.Errorf("first tab sort = %q, want ↑ number", label)
	}
}

func TestSortOrdersMarkSections(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12):  "wip",
		markKey("rivka/dotfiles", 3): "wip",
	})
	last := func() []string { rows := sectionRows(view); return rows[len(rows)-1] }
	// With GitHub's order a mark section is most recently updated first.
	if diff := cmp.Diff([]string{"acme/widgets#12", "rivka/dotfiles#3"}, last()); diff != "" {
		test.Errorf("default (-want +got):\n%s", diff)
	}
	sortBy(test, view, "number")
	if diff := cmp.Diff([]string{"rivka/dotfiles#3", "acme/widgets#12"}, last()); diff != "" {
		test.Errorf("sorted (-want +got):\n%s", diff)
	}
}

func TestSortErrorsAndCompletion(test *testing.T) {
	view, _ := loadedView(test)
	failure, ok := commandMessage(test, view, "sort", "age").(userinterface.ErrorMessage)
	if !ok || failure.Err.Error() != "unknown sort key: age" {
		test.Errorf("message = %#v", failure)
	}
	want := []string{"number", "title", "author", "created", "updated", "review", "checks", "comments", "draft", "default"}
	if diff := cmp.Diff(want, view.CompleteArguments("sort")); diff != "" {
		test.Errorf("completion (-want +got):\n%s", diff)
	}
}

func TestSortGolden(test *testing.T) {
	for _, testCase := range []struct {
		name          string
		width         int
		wantIndicator bool
	}{
		{"sorted", 80, true},
		{"sorted_narrow", 30, true},
		{"sorted_too_narrow", 20, false},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			view, _ := loadedView(test)
			sortBy(test, view, "comments desc")
			out := view.Render(testCase.width, 12)
			for i, line := range strings.Split(out, "\n") {
				if width := ansi.StringWidth(line); width > testCase.width {
					test.Errorf("line %d is %d columns wide, max %d", i, width, testCase.width)
				}
			}
			tabBar := ansi.Strip(strings.Split(out, "\n")[0])
			if hasArrow := strings.Contains(tabBar, "↓ comments"); hasArrow != testCase.wantIndicator {
				test.Errorf("tab bar %q: indicator shown = %v, want %v", tabBar, hasArrow, testCase.wantIndicator)
			}
			golden.Assert(test, testCase.name, out, false)
		})
	}
}
