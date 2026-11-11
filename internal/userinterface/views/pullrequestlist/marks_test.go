package pullrequestlist

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

func markedView(test *testing.T, pullRequests []github.PullRequest) *View {
	test.Helper()
	return loadedView(test, hostedClient{Client: fakeWith(pullRequests), host: "github.com"}, nil)
}

func markKey(number int) pullrequestmarks.Key {
	return pullrequestmarks.Key{Host: "github.com", Repository: testRepository, Number: number}
}

func withMarks(test *testing.T, view *View, texts map[int]string) {
	test.Helper()
	var marks pullrequestmarks.Marks
	for number, text := range texts {
		var err error
		if marks, err = marks.Set(markKey(number), text); err != nil {
			test.Fatal(err)
		}
	}
	view.Update(userinterface.MarksMessage{Marks: marks})
}

// commandMessage runs a view command and returns the one message it produced.
func commandMessage(test *testing.T, view *View, name, arguments string) tea.Msg {
	test.Helper()
	command, handled := view.Command(name, arguments)
	if !handled || command == nil {
		test.Fatalf("%s not handled", name)
	}
	return command()
}

func TestMarkAsksTheRootToMarkTheRowUnderTheCursor(test *testing.T) {
	view := markedView(test, makePullRequests(3))
	press(view, "j")
	got := commandMessage(test, view, "mark", "waiting on author")
	want := userinterface.SetMarkMessage{Key: markKey(2), Text: "waiting on author"}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("message (-want +got):\n%s", diff)
	}
}

func TestMarkErrors(test *testing.T) {
	tests := []struct {
		name      string
		setup     func(test *testing.T) *View
		arguments string
		want      string
	}{
		{"no text", func(test *testing.T) *View { return markedView(test, makePullRequests(2)) }, "", ":mark needs a name"},
		{"no row", func(test *testing.T) *View { return markedView(test, nil) }, "wip", "no pull request to mark"},
		{"no host", func(test *testing.T) *View { return loadedView(test, fakeWith(makePullRequests(2)), nil) }, "wip", "not marked: the host of this list is unknown"},
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
	view := markedView(test, makePullRequests(3))
	withMarks(test, view, map[int]string{3: "wip"})

	if diff := cmp.Diff(userinterface.ClearMarkMessage{Key: markKey(3)}, commandMessage(test, view, "unmark", "")); diff != "" {
		test.Errorf("marked row (-want +got):\n%s", diff)
	}
	press(view, "j")
	failure, ok := commandMessage(test, view, "unmark", "").(userinterface.ErrorMessage)
	if !ok || failure.Err.Error() != "#2 has no mark" {
		test.Errorf("unmarked row: message = %#v", failure)
	}
	unhosted := loadedView(test, fakeWith(makePullRequests(2)), nil)
	failure, ok = commandMessage(test, unhosted, "unmark", "").(userinterface.ErrorMessage)
	if !ok || failure.Err.Error() != "not unmarked: the host of this list is unknown" {
		test.Errorf("no host: message = %#v", failure)
	}
	empty := markedView(test, nil)
	failure, ok = commandMessage(test, empty, "unmark", "").(userinterface.ErrorMessage)
	if !ok || failure.Err.Error() != "no pull request to unmark" {
		test.Errorf("no row: message = %#v", failure)
	}
}

func TestMarkCompletionOffersTheMarksInUse(test *testing.T) {
	view := markedView(test, makePullRequests(3))
	withMarks(test, view, map[int]string{3: "waiting on author", 2: "left for Sam", 1: "waiting on author"})
	want := []string{"left for Sam", "waiting on author"}
	if diff := cmp.Diff(want, view.CompleteArguments("mark")); diff != "" {
		test.Errorf("mark (-want +got):\n%s", diff)
	}
	if got := view.CompleteArguments("unmark"); got != nil {
		test.Errorf("unmark offered %q", got)
	}
}

func TestMarkColumnFollowsMarksMessage(test *testing.T) {
	view := markedView(test, makePullRequests(3))
	if out := ansi.Strip(view.Render(80, 8)); strings.Contains(out, "MARK") {
		test.Errorf("column shown with no marks:\n%s", out)
	}
	withMarks(test, view, map[int]string{2: "waiting on author, then more"})
	lines := strings.Split(ansi.Strip(view.Render(80, 8)), "\n")
	if !strings.Contains(lines[1], "MARK") || !strings.Contains(lines[3], "waiting on auth…") {
		test.Errorf("marked list:\n%s", strings.Join(lines, "\n"))
	}
	withMarks(test, view, nil)
	if out := ansi.Strip(view.Render(80, 8)); strings.Contains(out, "MARK") {
		test.Errorf("column still shown after the marks went:\n%s", out)
	}
}

func TestMarkColumnIsNotMixedUpWithAnotherHost(test *testing.T) {
	view := markedView(test, makePullRequests(2))
	marks, err := pullrequestmarks.Marks{}.Set(pullrequestmarks.Key{Host: "ghe.example.test", Repository: testRepository, Number: 2}, "other host")
	if err != nil {
		test.Fatal(err)
	}
	view.Update(userinterface.MarksMessage{Marks: marks})
	if out := ansi.Strip(view.Render(80, 8)); strings.Contains(out, "other host") {
		test.Errorf("a mark on another host is shown:\n%s", out)
	}
}

func TestMarkGolden(test *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{"marked", 140, 12},
		{"marked_narrow", 118, 12},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := markedView(test, goldenPullRequests())
			withMarks(test, view, map[int]string{8: "waiting on author", 6: "left for Sam", 4: "a mark that is much too long to show"})
			press(view, "j")
			out := view.Render(testCase.width, testCase.height)
			for i, line := range strings.Split(out, "\n") {
				if width := ansi.StringWidth(line); width > testCase.width {
					test.Errorf("line %d is %d columns wide, max %d", i, width, testCase.width)
				}
			}
			golden.Assert(test, testCase.name, out, false)
		})
	}
}
