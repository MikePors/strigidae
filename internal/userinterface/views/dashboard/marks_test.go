package dashboard

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/pullrequestmarks"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// hostedClient is the fake with the host the rows came from, which the real
// client reports.
type hostedClient struct {
	*fake.Client
	host string
}

func (client hostedClient) Host() string { return client.host }

func markedView(test *testing.T, host string) *View {
	test.Helper()
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(hostedClient{Client: client, host: host}, tabs, nil)
	settle(view, view.Init())
	return view
}

func markKey(repository string, number int) pullrequestmarks.Key {
	parsed, err := github.ParseRepository(repository)
	if err != nil {
		panic(err)
	}
	return pullrequestmarks.Key{Host: "github.com", Repository: parsed, Number: number}
}

func withMarks(test *testing.T, view *View, texts map[pullrequestmarks.Key]string) {
	test.Helper()
	var marks pullrequestmarks.Marks
	for key, text := range texts {
		var err error
		if marks, err = marks.Set(key, text); err != nil {
			test.Fatal(err)
		}
	}
	view.Update(userinterface.MarksMessage{Marks: marks})
}

func commandMessage(test *testing.T, view *View, name, arguments string) any {
	test.Helper()
	command, handled := view.Command(name, arguments)
	if !handled || command == nil {
		test.Fatalf("%s not handled", name)
	}
	return command()
}

func TestMarkActsOnTheRowUnderTheCursorInAnySection(test *testing.T) {
	tests := []struct {
		name string
		keys []string
		want pullrequestmarks.Key
	}{
		{"first row", nil, markKey("acme/widgets", 12)},
		{"second row", []string{"j"}, markKey("acme/gadgets", 7)},
		{"second section", []string{"]"}, markKey("rivka/dotfiles", 3)},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := markedView(test, "github.com")
			for _, sequence := range testCase.keys {
				press(view, sequence)
			}
			want := userinterface.SetMarkMessage{Key: testCase.want, Text: "waiting on author"}
			if diff := cmp.Diff(want, commandMessage(test, view, "mark", "waiting on author")); diff != "" {
				test.Errorf("message (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMarkErrors(test *testing.T) {
	tests := []struct {
		name      string
		setup     func(test *testing.T) *View
		arguments string
		want      string
	}{
		{"no text", func(test *testing.T) *View { return markedView(test, "github.com") }, "", ":mark needs a name"},
		{"no row", func(test *testing.T) *View {
			view := newView(fake.New(), testTabs(test), nil)
			settle(view, view.Init())
			return view
		}, "wip", "no pull request to mark"},
		{"no host", func(test *testing.T) *View { return markedView(test, "") }, "wip", "not marked: the host of this list is unknown"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			failure, ok := commandMessage(test, testCase.setup(test), "mark", testCase.arguments).(userinterface.ErrorMessage)
			if !ok || failure.Err.Error() != testCase.want {
				test.Errorf("message = %#v, want error %q", failure, testCase.want)
			}
		})
	}
}

func TestUnmark(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("rivka/dotfiles", 3): "wip"})

	failure, ok := commandMessage(test, view, "unmark", "").(userinterface.ErrorMessage)
	if !ok || failure.Err.Error() != "#12 has no mark" {
		test.Errorf("unmarked row: message = %#v", failure)
	}
	press(view, "]")
	if diff := cmp.Diff(userinterface.ClearMarkMessage{Key: markKey("rivka/dotfiles", 3)}, commandMessage(test, view, "unmark", "")); diff != "" {
		test.Errorf("marked row (-want +got):\n%s", diff)
	}
	empty := newView(fake.New(), testTabs(test), nil)
	settle(empty, empty.Init())
	failure, ok = commandMessage(test, empty, "unmark", "").(userinterface.ErrorMessage)
	if !ok || failure.Err.Error() != "no pull request to unmark" {
		test.Errorf("no row: message = %#v", failure)
	}
}

func TestMarkCompletionOffersTheMarksInUse(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12): "waiting on author",
		markKey("acme/gadgets", 7):  "left for Sam",
	})
	if diff := cmp.Diff([]string{"left for Sam", "waiting on author"}, view.CompleteArguments("mark")); diff != "" {
		test.Errorf("mark (-want +got):\n%s", diff)
	}
	if got := view.CompleteArguments("copy-url"); got != nil {
		test.Errorf("copy-url offered %q", got)
	}
}

func TestMarkColumnFollowsMarksMessage(test *testing.T) {
	view := markedView(test, "github.com")
	if out := ansi.Strip(view.Render(100, 12)); strings.Contains(out, "MARK") {
		test.Errorf("column shown with no marks:\n%s", out)
	}
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("rivka/dotfiles", 3): "left for Sam"})
	out := ansi.Strip(view.Render(100, 12))
	if !strings.Contains(out, "MARK") || !strings.Contains(out, "left for Sam") {
		test.Errorf("marked dashboard:\n%s", out)
	}
	withMarks(test, view, nil)
	if out := ansi.Strip(view.Render(100, 12)); strings.Contains(out, "MARK") {
		test.Errorf("column still shown after the marks went:\n%s", out)
	}
}

func TestMarkGolden(test *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{"marked", 100, 12},
		{"marked_narrow", 80, 12},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := markedView(test, "github.com")
			withMarks(test, view, map[pullrequestmarks.Key]string{
				markKey("acme/widgets", 12):  "waiting on author",
				markKey("rivka/dotfiles", 3): "a mark that is much too long to show",
			})
			press(view, "j")
			out := ansi.Strip(view.Render(testCase.width, testCase.height))
			for i, line := range strings.Split(out, "\n") {
				if width := ansi.StringWidth(line); width > testCase.width {
					test.Errorf("line %d is %d columns wide, max %d", i, width, testCase.width)
				}
			}
			golden.Assert(test, testCase.name, out, false)
		})
	}
}

func TestMarkSections(test *testing.T) {
	view := markedView(test, "github.com")
	current := &view.tabs[0]
	duplicate := current.sections[0].pullRequests[0]
	current.sections[1].pullRequests = append(current.sections[1].pullRequests, duplicate)
	current.selectIndex(0)
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12):      "later",
		markKey("rivka/dotfiles", 3):     "ahead",
		markKey("absent/repository", 99): "invisible",
	})
	sections := current.displaySections()
	if len(sections) != len(current.sections)+2 {
		test.Fatalf("got %d sections", len(sections))
	}
	if sections[len(sections)-2].name != "ahead" || sections[len(sections)-1].name != "later" {
		test.Fatal("mark sections are not sorted")
	}
	if current.total() != 3 {
		test.Fatalf("got %d rows, want 3 unique rows", current.total())
	}
	selected, _ := current.selectedPullRequest()
	if selected.Number != 7 {
		test.Fatalf("cursor followed marked row: %d", selected.Number)
	}
	current.selectIndex(current.total() - 1)
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("rivka/dotfiles", 3): "ahead"})
	if _, _, ok := current.position(); !ok {
		test.Fatal("cursor lost after removing last mark section")
	}
	withMarks(test, view, nil)
	if len(current.displaySections()) != len(current.sections) {
		test.Fatal("empty mark sections remain")
	}
	if len(current.sections[1].pullRequests) != 2 {
		test.Fatal("loaded rows mutated")
	}
}

func TestMarkCursorKeepsRow(test *testing.T) {
	view := markedView(test, "github.com")
	current := &view.tabs[0]
	current.selectIndex(0)
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("acme/widgets", 12): "later"})
	section, row, _ := current.position()
	if section != 0 || row != 0 {
		test.Fatalf("cursor = %d/%d", section, row)
	}
	withMarks(test, view, nil)
	section, row, _ = current.position()
	if section != 0 || row != 1 {
		test.Fatalf("cursor should retain its selected pull request after unrelated unmark: %d/%d", section, row)
	}
}

func TestMarkSectionsUseEachTabsRows(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12): "later",
		markKey("acme/gadgets", 7):  "later",
		markKey("acme/widgets", 40): "teammate",
	})
	current := &view.tabs[0]
	sections := current.displaySections()
	for _, target := range sections {
		if target.name == "teammate" {
			test.Fatal("other tab mark leaked")
		}
	}
	rows := sections[len(sections)-1].pullRequests
	if len(rows) != 2 || rows[0].Number != 12 || rows[1].Number != 7 {
		test.Fatalf("mark rows not newest first: %v", rows)
	}
	if len(current.searchList().Items()) != current.total() {
		test.Fatal("search does not see all displayed rows")
	}
	settle(view, view.switchTab(1))
	sections = view.tabs[1].displaySections()
	if len(sections) != len(view.tabs[1].sections)+1 || sections[len(sections)-1].name != "teammate" {
		test.Fatal("teammate tab does not derive its own mark sections")
	}
}

func TestUnmarkKeepsCursorInMarkSection(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12): "later",
		markKey("acme/gadgets", 7):  "later",
	})
	current := &view.tabs[0]
	current.selectIndex(1)
	section, row, _ := current.position()
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("acme/gadgets", 7): "later"})
	gotSection, gotRow, _ := current.position()
	selected, _ := current.selectedPullRequest()
	if gotSection != section || gotRow != row || selected.Number != 7 {
		test.Fatalf("unmark cursor = %d/%d #%d, want %d/%d #7", gotSection, gotRow, selected.Number, section, row)
	}
}

func TestMarkSectionIdentity(test *testing.T) {
	for _, rename := range []bool{false, true} {
		view := markedView(test, "github.com")
		withMarks(test, view, map[pullrequestmarks.Key]string{
			markKey("acme/widgets", 12): "later",
			markKey("acme/gadgets", 7):  "later",
		})
		current := &view.tabs[0]
		current.selectIndex(1)
		marks := map[pullrequestmarks.Key]string{
			markKey("acme/widgets", 12):  "later",
			markKey("acme/gadgets", 7):   "later",
			markKey("rivka/dotfiles", 3): "ahead",
		}
		want := 12
		if rename {
			marks[markKey("acme/widgets", 12)] = "aaa"
			want = 7
		}
		withMarks(test, view, marks)
		selected, _ := current.selectedPullRequest()
		section, _, _ := current.position()
		if selected.Number != want || current.displaySections()[section].mark != "later" {
			test.Fatalf("rename %v: cursor on #%d in %q", rename, selected.Number, current.displaySections()[section].mark)
		}
	}
}

func TestUnrelatedMarksKeepUntouchedCursor(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("absent/repo", 99): "later"})
	if view.tabs[0].anchor != nil {
		test.Fatal("untouched cursor was anchored")
	}
}

func TestMarkNavigationAndSearch(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12):  "ahead",
		markKey("rivka/dotfiles", 3): "later",
	})
	pressCounted(view, "]", 2)
	if got := selectedKey(test, view); got != "rivka/dotfiles#3" {
		test.Fatal(got)
	}
	pressCounted(view, "k", 2)
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Fatal(got)
	}
	press(view, "/")
	promptResult(view, "acme", userinterface.PromptChanged)
	promptResult(view, "acme", userinterface.PromptSubmitted)
	press(view, "n")
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Fatal(got)
	}
	press(view, "N")
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Fatal(got)
	}
}

func TestReloadInsertsEarlierMarkSection(test *testing.T) {
	view := markedView(test, "github.com")
	current := &view.tabs[0]
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12): "later",
		markKey("acme/gadgets", 7):  "later",
		markKey("acme/widgets", 99): "ahead",
	})
	current.selectIndex(1)
	target := &current.sections[0]
	rows := append([]github.PullRequest(nil), target.pullRequests...)
	added := rows[0]
	added.Number = 99
	rows = append(rows, added)
	view.loaded(sectionLoadedMessage{tab: 0, section: 0, sequence: target.sequence, page: github.Page[github.PullRequest]{Items: rows}})
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Fatal(got)
	}
	view.loaded(sectionLoadedMessage{tab: 0, section: 0, sequence: target.sequence, page: github.Page[github.PullRequest]{Items: rows[:2]}})
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Fatal(got)
	}
}

func TestLastMarkSectionFallsBackToLastRow(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("acme/widgets", 12): "later"})
	press(view, "G")
	withMarks(test, view, nil)
	if got := selectedKey(test, view); got != "rivka/dotfiles#3" {
		test.Fatal(got)
	}
}

func TestMarkedRowsRespectSectionLimit(test *testing.T) {
	tabs := testTabs(test)
	tabs[0].Sections[0].Limit = 1
	client := fake.New()
	seed(test, client, tabs)
	view := newView(hostedClient{Client: client, host: "github.com"}, tabs, nil)
	withMarks(test, view, map[pullrequestmarks.Key]string{markKey("acme/gadgets", 7): "excluded"})
	settle(view, view.Init())
	for _, target := range view.tabs[0].displaySections() {
		if target.mark == "excluded" {
			test.Fatal("row beyond configured limit appeared")
		}
	}
}

func TestMarkSectionsPreserveLoadStatus(test *testing.T) {
	view := markedView(test, "github.com")
	current := &view.tabs[0]
	current.sections[0].stale = true
	current.sections[0].err = errors.New("offline")
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12): "later",
		markKey("acme/gadgets", 7):  "later",
	})
	sections := current.displaySections()
	target := sections[len(sections)-1]
	if !target.stale || target.err == nil {
		test.Fatal("mark section lost source status")
	}
	output := ansi.Strip(view.Render(120, 30))
	if !strings.Contains(output, "later (stale) (refresh failed: offline)") {
		test.Fatal(output)
	}
}

func TestMarkNamesHaveDeterministicCaseSensitiveOrder(test *testing.T) {
	view := markedView(test, "github.com")
	withMarks(test, view, map[pullrequestmarks.Key]string{
		markKey("acme/widgets", 12): "apple",
		markKey("acme/gadgets", 7):  "Zed",
	})
	sections := view.tabs[0].displaySections()
	if sections[len(sections)-2].mark != "Zed" || sections[len(sections)-1].mark != "apple" {
		test.Fatal("unexpected mark name ordering")
	}
}
