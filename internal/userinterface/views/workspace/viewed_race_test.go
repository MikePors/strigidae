package workspace

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

// pressV delivers v and returns its command without running it, so a test
// decides when the mutation reaches the server.
func (harness *harness) pressV() tea.Cmd {
	harness.test.Helper()
	view, command := harness.view.Update(keys.Message{Sequence: "v"})
	harness.view = view.(*View)
	return command
}

// finish runs command, delivers its results and returns the follow-up command the
// view issued, if any.
func (harness *harness) finish(command tea.Cmd) tea.Cmd {
	harness.test.Helper()
	var next tea.Cmd
	for _, message := range messages(command) {
		switch message.(type) {
		case userinterface.ErrorMessage, userinterface.InfoMessage:
			harness.messages = append(harness.messages, message)
		default:
			view, command := harness.view.Update(message)
			harness.view = view.(*View)
			if command != nil {
				next = command
			}
		}
	}
	return next
}

func sentStates(client *fake.Client) []bool {
	var out []bool
	for _, call := range viewedCalls(client) {
		out = append(out, call.Viewed)
	}
	return out
}

func (harness *harness) reload(detail github.PullRequestDetail) {
	harness.test.Helper()
	harness.send(pullRequestLoadedMessage{owner: harness.view, repository: testRepository, number: 7, sequence: harness.view.sequence, detail: detail})
}

func TestRapidTogglesAreSerialised(test *testing.T) {
	tests := []struct {
		name       string
		presses    int
		wantSent   []bool
		wantServer github.ViewedState
	}{
		{"two presses", 2, []bool{true, false}, github.FileUnviewed},
		{"three presses", 3, []bool{true}, github.FileViewed},
		{"four presses", 4, []bool{true, false}, github.FileUnviewed},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := viewedHarness(test, 100, 24)
			harness.press("j")
			command := harness.pressV()
			for range testCase.presses - 1 {
				if harness.pressV() != nil {
					test.Fatal("a second mutation was sent while one is in flight")
				}
			}
			if count := len(viewedCalls(harness.client)); count != 0 {
				test.Fatalf("%d calls before the first result", count)
			}
			for command != nil {
				command = harness.finish(command)
			}
			if got := sentStates(harness.client); !slices.Equal(got, testCase.wantSent) {
				test.Errorf("sent = %v, want %v", got, testCase.wantSent)
			}
			if got := harness.serverState(); got != testCase.wantServer {
				test.Errorf("server = %q, want %q", got, testCase.wantServer)
			}
			if got := harness.stateOf(); got != testCase.wantServer {
				test.Errorf("shown = %q, want %q", got, testCase.wantServer)
			}
		})
	}
}

func TestRapidTogglesBothFailRevertToConfirmed(test *testing.T) {
	harness := viewedHarness(test, 100, 24, github.FileUnviewed)
	harness.client.SetError(fake.MethodSetFileViewed, errors.New("boom"))
	harness.press("j")
	command := harness.pressV()
	harness.pressV() // wants unviewed again
	if next := harness.finish(command); next != nil {
		test.Error("nothing more should be sent: the wanted state is the confirmed one")
	}
	if got := harness.stateOf(); got != github.FileUnviewed {
		test.Errorf("shown = %q", got)
	}
	if count := len(viewedCalls(harness.client)); count != 1 {
		test.Errorf("%d calls, want 1", count)
	}
	if harness.lastErr() == nil {
		test.Error("no error shown")
	}
}

func TestFailureMessagesNameTheAction(test *testing.T) {
	harness := viewedHarness(test, 100, 24, github.FileViewed)
	harness.client.SetError(fake.MethodSetFileViewed, errors.New("boom"))
	harness.press("j", "v")
	err := harness.lastErr()
	if err == nil || !strings.HasPrefix(err.Error(), "unmark "+firstFile+" as viewed: boom") {
		test.Errorf("error = %v", err)
	}
}

func TestRefreshMidToggleKeepsOptimisticMark(test *testing.T) {
	harness := viewedHarness(test, 100, 24)
	harness.press("j")
	command := harness.pressV()
	harness.reload(testDetail()) // read before the mutation landed: unviewed
	if got := harness.stateOf(); got != github.FileViewed {
		test.Fatalf("refresh dropped the optimistic mark: %q", got)
	}
	harness.finish(command)
	if got := harness.stateOf(); got != github.FileViewed {
		test.Errorf("after success shown = %q", got)
	}
	// Once settled, the next reload shows the server's state again.
	harness.view.sequence++ // a reload started after the mutation settled
	detail := testDetail()
	detail.Files[0].Viewed = github.FileDismissed
	harness.reload(detail)
	if got := harness.stateOf(); got != github.FileDismissed {
		test.Errorf("settled entry still overrides the reload: %q", got)
	}
	if len(harness.view.viewedState) != 0 {
		test.Errorf("settled entries kept: %v", harness.view.viewedState)
	}
}

func TestFailureAfterRefreshReverts(test *testing.T) {
	harness := viewedHarness(test, 100, 24, github.FileUnviewed)
	harness.client.SetError(fake.MethodSetFileViewed, errors.New("boom"))
	harness.press("j")
	command := harness.pressV()
	harness.reload(unviewedDetail())
	harness.finish(command)
	if got := harness.stateOf(); got != github.FileUnviewed {
		test.Errorf("shown = %q, want reverted", got)
	}
	// The next reload has nothing left to reapply.
	harness.reload(unviewedDetail())
	if got := harness.stateOf(); got != github.FileUnviewed {
		test.Errorf("shown = %q", got)
	}
}

func TestToggleOnVanishedFileIsDropped(test *testing.T) {
	harness := viewedHarness(test, 100, 24)
	harness.press("j")
	command := harness.pressV()
	detail := testDetail()
	detail.Files = detail.Files[1:]
	harness.reload(detail)
	if len(harness.view.viewedState) != 0 {
		test.Errorf("entry for a vanished file kept: %v", harness.view.viewedState)
	}
	harness.finish(command) // must not panic or resurrect anything
	if harness.view.viewedCount() != 0 {
		test.Error("a mark appeared on another file")
	}
}

func TestViewedResultForOtherViewIgnored(test *testing.T) {
	tests := []struct {
		name   string
		mutate func(message *viewedMessage)
	}{
		{"other number", func(message *viewedMessage) { message.number = 8 }},
		{"other repo", func(message *viewedMessage) { message.repository = github.Repository{Owner: "x", Name: "y"} }},
		{"other view", func(message *viewedMessage) { message.owner = New(nil, testRepository, 7) }},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := viewedHarness(test, 100, 24)
			harness.press("j")
			harness.pressV()
			message := viewedMessage{owner: harness.view, repository: testRepository, number: 7, path: firstFile, sent: github.FileViewed, err: errors.New("x")}
			testCase.mutate(&message)
			harness.send(message)
			if got := harness.stateOf(); got != github.FileViewed {
				test.Errorf("foreign result changed state to %q", got)
			}
		})
	}
}

func TestReloadStartedBeforeSettleKeepsMark(test *testing.T) {
	harness := viewedHarness(test, 100, 24, github.FileUnviewed)
	harness.press("j")
	command := harness.pressV()
	harness.finish(command) // settles at the current load seq
	// A reload that began before the settle finishes with the old data.
	harness.reload(unviewedDetail())
	if got := harness.stateOf(); got != github.FileViewed {
		test.Fatalf("stale reload overwrote the settled mark: %q", got)
	}
	// A reload started afterwards (a newer seq) shows the server's state.
	harness.view.sequence++
	detail := unviewedDetail()
	detail.Files[0].Viewed = github.FileDismissed
	harness.reload(detail)
	if got := harness.stateOf(); got != github.FileDismissed {
		test.Errorf("later reload not shown: %q", got)
	}
}

func unviewedDetail() github.PullRequestDetail {
	detail := testDetail()
	detail.Files[0].Viewed = github.FileUnviewed
	return detail
}
