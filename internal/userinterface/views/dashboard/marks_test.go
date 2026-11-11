package dashboard

import (
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
