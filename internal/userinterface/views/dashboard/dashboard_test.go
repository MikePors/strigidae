package dashboard

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/views/workspace"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

const teammateConfig = `tabs:
  - name: Me
  - name: Rivka
    user: rivka
    sections:
      - {name: Needs review, query: 'review-requested:{user}', limit: 5}
`

func testTabs(test *testing.T) []config.Tab {
	test.Helper()
	configuration, err := config.Parse("config.yml", []byte(teammateConfig))
	if err != nil {
		test.Fatal(err)
	}
	return configuration.Tabs
}

func pullRequest(repository string, number int, title string, age time.Duration) github.PullRequest {
	parsed, err := github.ParseRepository(repository)
	if err != nil {
		panic(err)
	}
	return github.PullRequest{
		Repository: parsed,
		Number:     number,
		Title:      title,
		Author:     "octo-dev",
		State:      github.PullRequestStateOpen,
		URL:        fmt.Sprintf("https://github.com/%s/pull/%d", repository, number),
		UpdatedAt:  testNow.Add(-age),
	}
}

// seed fills the fake: two Me sections with rows, one empty, and a teammate section.
func seed(test *testing.T, client *fake.Client, tabs []config.Tab) {
	test.Helper()
	review := pullRequest("acme/widgets", 12, "Cache the parsed diff", time.Hour)
	review.ReviewDecision = github.ReviewDecisionReviewRequired
	review.Checks = github.ChecksPassing
	other := pullRequest("acme/gadgets", 7, "Fix a very long title that will certainly need to be cut off somewhere", 3*time.Hour)
	other.Draft = true
	mine := pullRequest("rivka/dotfiles", 3, "Tidy the shell prompt", 26*time.Hour)
	mine.CommentCount = 4
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{review, other})
	client.SetSearchResults(searchQuery(tabs[0].Sections[1]), []github.PullRequest{mine})
	client.SetSearchResults(searchQuery(tabs[1].Sections[0]), []github.PullRequest{pullRequest("acme/widgets", 40, "Rivka's refactor", 5*time.Minute)})
}

func newView(client github.Client, tabs []config.Tab, open func(string) error) *View {
	view := New(client, tabs, theme.Default(), Options{Now: func() time.Time { return testNow }, Open: open})
	view.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	return view
}

// messages runs command and flattens batches into the messages produced.
func messages(command tea.Cmd) []tea.Msg {
	if command == nil {
		return nil
	}
	message := command()
	if batch, ok := message.(tea.BatchMsg); ok {
		var all []tea.Msg
		for _, inner := range batch {
			all = append(all, messages(inner)...)
		}
		return all
	}
	return []tea.Msg{message}
}

func deliver(view *View, all []tea.Msg) {
	for _, message := range all {
		view.Update(message)
	}
}

func settle(view *View, command tea.Cmd) { deliver(view, messages(command)) }

func press(view *View, sequence string) tea.Cmd {
	_, command := view.Update(keys.Message{Sequence: sequence})
	return command
}

func loadedView(test *testing.T) (*View, *fake.Client) {
	test.Helper()
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	return view, client
}

func searches(client *fake.Client) int {
	count := 0
	for _, call := range client.Calls() {
		if call.Method == fake.MethodSearchPullRequests {
			count++
		}
	}
	return count
}

func selectedKey(test *testing.T, view *View) string {
	test.Helper()
	pullRequest, ok := view.selected()
	if !ok {
		test.Fatal("nothing selected")
	}
	return fmt.Sprintf("%s#%d", pullRequest.Repository, pullRequest.Number)
}

func TestTabKeysWrapAround(test *testing.T) {
	view, _ := loadedView(test)
	steps := []struct {
		sequence string
		want     int
	}{{"g t", 1}, {"g t", 0}, {"g T", 1}, {"g T", 0}}
	for _, step := range steps {
		press(view, step.sequence)
		if view.active != step.want {
			test.Fatalf("after %q active tab = %d, want %d", step.sequence, view.active, step.want)
		}
	}
}

func TestSectionKeys(test *testing.T) {
	view, _ := loadedView(test)
	want := []struct {
		sequence string
		want     string
	}{
		{"", "acme/widgets#12"},
		{"j", "acme/gadgets#7"},
		{"[", "acme/gadgets#7"}, // no earlier section with rows
		{"]", "rivka/dotfiles#3"},
		{"]", "rivka/dotfiles#3"}, // the last section is empty: nowhere to go
		{"[", "acme/widgets#12"},
		{"G", "rivka/dotfiles#3"},
		{"k", "acme/gadgets#7"},
		{"g g", "acme/widgets#12"},
	}
	for _, step := range want {
		if step.sequence != "" {
			press(view, step.sequence)
		}
		if got := selectedKey(test, view); got != step.want {
			test.Fatalf("after %q selected %s, want %s", step.sequence, got, step.want)
		}
	}
}

func TestTabsLoadOnFirstShow(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(client, tabs, nil)

	settle(view, view.Init())
	if got := searches(client); got != 3 {
		test.Fatalf("searches after Init = %d, want the 3 sections of the first tab", got)
	}
	settle(view, press(view, "g t"))
	if got := searches(client); got != 4 {
		test.Fatalf("searches after showing tab 2 = %d, want 4", got)
	}
	settle(view, press(view, "g t")) // back to the first tab
	settle(view, press(view, "g t")) // and to the second again
	settle(view, press(view, "g t"))
	if got := searches(client); got != 4 {
		test.Errorf("switching back and forth searched again: %d, want 4", got)
	}
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("results of the first tab were lost: selected %s", got)
	}
	last := client.Calls()[3]
	if want := "is:pr review-requested:rivka sort:updated-desc"; last.Query != want || last.Page.Limit != 5 {
		test.Errorf("teammate search = %q limit %d, want %q limit 5", last.Query, last.Page.Limit, want)
	}
}

func TestEnterPushesWorkspaceForTheRow(test *testing.T) {
	view, client := loadedView(test)
	press(view, "j") // acme/gadgets#7
	push, ok := press(view, "enter")().(userinterface.PushMessage)
	if !ok {
		test.Fatal("enter should push a view")
	}
	if got := push.View.Title(); got != "#7" {
		test.Errorf("pushed view title = %q, want #7", got)
	}
	messages(push.View.Init()) // the workspace starts loading the row's pull request
	var got []github.Repository
	for _, call := range client.Calls() {
		if call.Method == fake.MethodGetPullRequest && call.Number == 7 {
			got = append(got, call.Repository)
		}
	}
	if diff := cmp.Diff([]github.Repository{{Owner: "acme", Name: "gadgets"}}, got); diff != "" {
		test.Errorf("workspace loaded the wrong repository (-want +got):\n%s", diff)
	}

	empty := newView(fake.New(), testTabs(test), nil)
	if press(empty, "enter") != nil {
		test.Error("enter with no rows should do nothing")
	}
}

func TestOpenInBrowser(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	var opened []string
	view := newView(client, tabs, func(url string) error { opened = append(opened, url); return nil })
	settle(view, view.Init())
	press(view, "]")
	message := press(view, "o")()
	if _, ok := message.(userinterface.InfoMessage); !ok {
		test.Fatalf("o produced %T, want an info message", message)
	}
	if diff := cmp.Diff([]string{"https://github.com/rivka/dotfiles/pull/3"}, opened); diff != "" {
		test.Errorf("opened (-want +got):\n%s", diff)
	}
}

func TestStaleResultsAreIgnored(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(client, tabs, nil)

	first := messages(view.Init()) // in flight, not delivered yet
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{pullRequest("acme/widgets", 99, "Newer", time.Minute)})
	second := messages(press(view, "R"))
	deliver(view, second)
	deliver(view, first) // arrives late, with the old data

	if got := selectedKey(test, view); got != "acme/widgets#99" {
		test.Errorf("selected %s, want the refreshed acme/widgets#99", got)
	}
}

func TestRefreshKeepsRowsUntilNewOnesArrive(test *testing.T) {
	view, _ := loadedView(test)
	command := press(view, "R")
	if !view.Loading() {
		test.Error("refresh should show loading")
	}
	if out := ansi.Strip(view.Render(80, 16)); !strings.Contains(out, "Cache the parsed diff") {
		test.Errorf("rows disappeared during refresh:\n%s", out)
	}
	settle(view, command)
	if view.Loading() {
		test.Error("still loading after results arrived")
	}
}

func TestResultsForUnknownSectionsAreIgnored(test *testing.T) {
	view, _ := loadedView(test)
	before := ansi.Strip(view.Render(80, 16))
	selected := selectedKey(test, view)
	view.Update(sectionLoadedMessage{tab: 9, section: 0, sequence: 1, page: github.Page[github.PullRequest]{Items: []github.PullRequest{pullRequest("x/y", 1, "Bogus", 0)}}})
	view.Update(sectionLoadedMessage{tab: 0, section: 9, sequence: 1})
	view.Update(sectionLoadedMessage{tab: 0, section: 0, sequence: 99}) // not the latest request
	if after := ansi.Strip(view.Render(80, 16)); after != before {
		test.Errorf("a stray result changed the screen:\n%s\nwas:\n%s", after, before)
	}
	if got := selectedKey(test, view); got != selected {
		test.Errorf("selection changed to %s, was %s", got, selected)
	}
	if view.Loading() {
		test.Error("a stray result started loading")
	}
}

func TestOutOfOrderResultsKeepTheCursorAtTheTop(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(client, tabs, nil)
	all := messages(view.Init())
	for i := len(all) - 1; i >= 0; i-- { // the lowest section answers first
		view.Update(all[i])
	}
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("selected %s, want the first row of the first section", got)
	}
}

func TestRefreshKeepsTheCursorInItsSection(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	shared := pullRequest("acme/widgets", 5, "Listed twice", time.Hour)
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{shared})
	client.SetSearchResults(searchQuery(tabs[0].Sections[2]), []github.PullRequest{pullRequest("acme/widgets", 6, "Other", time.Hour), shared})
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	press(view, "G") // the copy in the third section
	settle(view, press(view, "R"))
	section, row, ok := view.tabs[0].position()
	if !ok || section != 2 || row != 1 {
		test.Errorf("after refresh the cursor is at section %d row %d, want section 2 row 1", section, row)
	}
}

func TestSwitchingTabsWhileTheFirstIsLoading(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(client, tabs, nil)
	first := messages(view.Init()) // in flight
	second := messages(press(view, "g t"))
	if !view.Loading() {
		test.Fatal("the second tab is loading, so the view should be")
	}
	deliver(view, first)
	if view.tabs[0].total() != 3 || view.tabs[1].total() != 0 {
		test.Errorf("rows landed in the wrong tab: %d and %d, want 3 and 0", view.tabs[0].total(), view.tabs[1].total())
	}
	if !view.Loading() {
		test.Error("Loading should follow the active tab, which has not answered")
	}
	deliver(view, second)
	if view.Loading() || view.tabs[1].total() != 1 {
		test.Errorf("second tab: loading %v, rows %d, want false and 1", view.Loading(), view.tabs[1].total())
	}
}

func TestRefreshOnlyReloadsTheActiveTab(test *testing.T) {
	view, client := loadedView(test)
	settle(view, press(view, "g t"))
	before := len(client.Calls())
	settle(view, press(view, "R"))
	calls := client.Calls()[before:]
	if len(calls) != 1 || calls[0].Query != "is:pr review-requested:rivka sort:updated-desc" {
		test.Errorf("refresh searched %+v, want only the teammate section", calls)
	}
}

func TestSectionKeysSkipSectionsWithoutRows(test *testing.T) {
	configured := func(name string) config.Section {
		return config.Section{Name: name, Query: "is:pr " + name, Limit: 5, Sort: config.SortUpdated, Order: config.OrderDescending}
	}
	tabs := []config.Tab{{Name: "T", Sections: []config.Section{configured("a"), configured("b"), configured("c"), configured("d")}}}
	client := fake.New()
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{pullRequest("o/a", 1, "A", 0)})
	client.SetSearchResults(searchQuery(tabs[0].Sections[3]), []github.PullRequest{pullRequest("o/d", 4, "D", 0)})
	view := newView(client, tabs, nil)
	all := messages(view.Init())
	failed := all[1].(sectionLoadedMessage)
	failed.err = errors.New("boom")
	deliver(view, []tea.Msg{all[0], failed, all[3]}) // section c is still loading

	out := ansi.Strip(view.Render(80, 16))
	for _, want := range []string{"Could not load: boom", "Loading..."} {
		if !strings.Contains(out, want) {
			test.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	press(view, "]")
	if got := selectedKey(test, view); got != "o/d#4" {
		test.Errorf("] selected %s, want o/d#4 (skipping the failed and loading sections)", got)
	}
	press(view, "[")
	if got := selectedKey(test, view); got != "o/a#1" {
		test.Errorf("[ selected %s, want o/a#1", got)
	}
}

func TestSectionHeadingStaysVisible(test *testing.T) {
	for _, keysPressed := range [][]string{{"G", "g g"}, {"G", "["}} {
		view, _ := loadedView(test)
		for _, sequence := range keysPressed {
			press(view, sequence)
			view.Render(80, 5)
		}
		if out := ansi.Strip(view.Render(80, 5)); !strings.Contains(out, "Review requested") {
			test.Errorf("after %v the first section's heading is off screen:\n%s", keysPressed, out)
		}
	}
}

func TestPagingUsesTheLastRenderedHeight(test *testing.T) {
	configured := config.Section{Name: "s", Query: "is:pr s", Limit: 30, Sort: config.SortUpdated, Order: config.OrderDescending}
	tabs := []config.Tab{{Name: "T", Sections: []config.Section{configured}}}
	var rows []github.PullRequest
	for i := range 30 {
		rows = append(rows, pullRequest("o/r", i+1, "Row", 0))
	}
	client := fake.New()
	client.SetSearchResults(searchQuery(configured), rows)
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	view.Update(tea.WindowSizeMsg{Width: 80, Height: 40}) // a resize must not decide the page size
	view.Render(80, 12)                                   // 10 list rows: half a page is 5
	press(view, "ctrl+d")
	if got := selectedKey(test, view); got != "o/r#6" {
		test.Errorf("ctrl+d selected %s, want o/r#6", got)
	}
	view.Render(80, 6) // 4 list rows: half a page is 2
	press(view, "ctrl+d")
	if got := selectedKey(test, view); got != "o/r#8" {
		test.Errorf("ctrl+d after a resize selected %s, want o/r#8", got)
	}
}

func TestFailedRefreshShowsTheReason(test *testing.T) {
	view, client := loadedView(test)
	client.SetError(fake.MethodSearchPullRequests, errors.New("boom\x1b[2J"))
	settle(view, press(view, "R"))
	out := ansi.Strip(view.Render(100, 16))
	if !strings.Contains(out, "refresh failed: boom") || !strings.Contains(out, "Cache the parsed diff") {
		test.Errorf("want the reason and the old rows:\n%s", out)
	}
	if strings.ContainsRune(out, 0x1b) {
		test.Errorf("escape reached the screen: %q", out)
	}
}

func TestNoticeIsShownOnStart(test *testing.T) {
	view := New(fake.New(), testTabs(test), theme.Default(), Options{Notice: "why we are here"})
	var got []string
	for _, message := range messages(view.Init()) {
		if info, ok := message.(userinterface.InfoMessage); ok {
			got = append(got, info.Text)
		}
	}
	if diff := cmp.Diff([]string{"why we are here"}, got); diff != "" {
		test.Errorf("info messages (-want +got):\n%s", diff)
	}
}

func TestHelpListsTheDashboardKeys(test *testing.T) {
	view, _ := loadedView(test)
	var got []string
	for _, group := range view.FullHelp() {
		for _, binding := range group {
			got = append(got, binding.Help().Key)
		}
	}
	if diff := cmp.Diff([]string{"gt", "gT", "]", "[", "/", "n", "N"}, got); diff != "" {
		test.Errorf("help keys (-want +got):\n%s", diff)
	}
}

func TestHostileTextIsNeutralised(test *testing.T) {
	bidiOverride := string(rune(0x202e))
	hostile := "\x1b[2J\x07" + bidiOverride + "\u009b"
	tabs := []config.Tab{{Name: "T" + hostile, Sections: []config.Section{{Name: "S" + hostile, Query: "is:pr q" + hostile, Limit: 5, Sort: config.SortUpdated, Order: config.OrderDescending}}}, {
		Name: "U", Sections: []config.Section{{Name: "V", Query: "is:pr v", Limit: 5, Sort: config.SortUpdated, Order: config.OrderDescending}},
	}}
	client := fake.New()
	row := pullRequest("acme/widgets", 1, "Title"+hostile, time.Hour)
	row.Repository.Owner = "ev" + hostile
	row.Author = "au" + hostile
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{row})
	client.SetError(fake.MethodSearchPullRequests, errors.New("boom"+hostile)) // overrides: first tab errors
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	check := func(label string) {
		for _, character := range strings.ReplaceAll(ansi.Strip(view.Render(80, 16)), "\n", "") {
			if character < ' ' || (character >= 0x7f && character <= 0x9f) || character == '‮' {
				test.Fatalf("%s: control rune %q reached the screen", label, character)
			}
		}
	}
	check("error")
	client.SetError(fake.MethodSearchPullRequests, nil)
	settle(view, press(view, "R"))
	check("rows")
}

func TestKeysWithoutTabs(test *testing.T) {
	view := newView(fake.New(), nil, nil)
	for _, sequence := range []string{"j", "g t", "]", "enter", "R"} {
		if press(view, sequence) != nil {
			test.Errorf("%q with no tabs should do nothing", sequence)
		}
	}
	if out := ansi.Strip(view.Render(40, 3)); !strings.Contains(out, "no tabs") {
		test.Errorf("render = %q", out)
	}
}

func TestGolden(test *testing.T) {
	tests := []struct {
		name          string
		setup         func(test *testing.T) *View
		width, height int
	}{
		{"loading", func(test *testing.T) *View {
			view := newView(fake.New(), testTabs(test), nil)
			view.Init()
			return view
		}, 80, 12},
		{"empty", func(test *testing.T) *View {
			view := newView(fake.New(), testTabs(test), nil)
			settle(view, view.Init())
			return view
		}, 80, 12},
		{"error", func(test *testing.T) *View {
			client := fake.New()
			client.SetError(fake.MethodSearchPullRequests, fmt.Errorf("search pull requests: %w", github.ErrNotFound))
			view := newView(client, testTabs(test), nil)
			settle(view, view.Init())
			return view
		}, 80, 12},
		{"populated", func(test *testing.T) *View {
			view, _ := loadedView(test)
			press(view, "j")
			return view
		}, 80, 12},
		{"second_tab", func(test *testing.T) *View {
			view, _ := loadedView(test)
			settle(view, press(view, "g t"))
			return view
		}, 80, 8},
		{"narrow_48", narrowView, 48, 12},
		{"narrow_40", narrowView, 40, 12},
		{"narrow_36", narrowView, 36, 12},
		{"narrow_30", narrowView, 30, 12},
		{"scrolled", func(test *testing.T) *View {
			view, _ := loadedView(test)
			press(view, "G")
			return view
		}, 80, 5},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := testCase.setup(test)
			out := ansi.Strip(view.Render(testCase.width, testCase.height))
			for number, line := range strings.Split(out, "\n") {
				if got := ansi.StringWidth(line); got > testCase.width {
					test.Errorf("line %d is %d columns wide, want at most %d", number, got, testCase.width)
				}
			}
			if got := strings.Count(out, "\n") + 1; got > testCase.height {
				test.Errorf("rendered %d lines, want at most %d", got, testCase.height)
			}
			golden.Assert(test, testCase.name, out, false)
		})
	}
}

func narrowView(test *testing.T) *View {
	view, _ := loadedView(test)
	press(view, "]")
	return view
}

// TestWorkspacesShareHunkMarks proves the dashboard hands every workspace it
// opens the same hunk marks, so marks survive closing and reopening a pull
// request.
func TestWorkspacesShareHunkMarks(test *testing.T) {
	view, _ := loadedView(test)
	opened := func() *workspace.HunkMarks {
		push, ok := press(view, "enter")().(userinterface.PushMessage)
		if !ok {
			test.Fatal("enter should push a view")
		}
		return push.View.(*workspace.View).HunkMarks()
	}
	first, second := opened(), opened()
	if first == nil || first != second || first != view.hunkMarks {
		test.Error("two workspaces opened from the dashboard do not share its hunk marks")
	}
}
