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
func (h *harness) pressV() tea.Cmd {
	h.t.Helper()
	v, cmd := h.v.Update(keys.Message{Sequence: "v"})
	h.v = v.(*View)
	return cmd
}

// finish runs cmd, delivers its results and returns the follow-up command the
// view issued, if any.
func (h *harness) finish(cmd tea.Cmd) tea.Cmd {
	h.t.Helper()
	var next tea.Cmd
	for _, m := range messages(cmd) {
		switch m.(type) {
		case userinterface.ErrorMessage, userinterface.InfoMessage:
			h.messages = append(h.messages, m)
		default:
			v, c := h.v.Update(m)
			h.v = v.(*View)
			if c != nil {
				next = c
			}
		}
	}
	return next
}

func sentStates(c *fake.Client) []bool {
	var out []bool
	for _, call := range viewedCalls(c) {
		out = append(out, call.Viewed)
	}
	return out
}

func (h *harness) reload(d github.PullRequestDetail) {
	h.t.Helper()
	h.send(pullRequestLoadedMessage{owner: h.v, repository: testRepository, number: 7, sequence: h.v.sequence, detail: d})
}

func TestRapidTogglesAreSerialised(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := viewedHarness(t, 100, 24)
			h.press("j")
			cmd := h.pressV()
			for range tt.presses - 1 {
				if h.pressV() != nil {
					t.Fatal("a second mutation was sent while one is in flight")
				}
			}
			if n := len(viewedCalls(h.client)); n != 0 {
				t.Fatalf("%d calls before the first result", n)
			}
			for cmd != nil {
				cmd = h.finish(cmd)
			}
			if got := sentStates(h.client); !slices.Equal(got, tt.wantSent) {
				t.Errorf("sent = %v, want %v", got, tt.wantSent)
			}
			if got := h.serverState(); got != tt.wantServer {
				t.Errorf("server = %q, want %q", got, tt.wantServer)
			}
			if got := h.stateOf(); got != tt.wantServer {
				t.Errorf("shown = %q, want %q", got, tt.wantServer)
			}
		})
	}
}

func TestRapidTogglesBothFailRevertToConfirmed(t *testing.T) {
	h := viewedHarness(t, 100, 24, github.FileUnviewed)
	h.client.SetError(fake.MethodSetFileViewed, errors.New("boom"))
	h.press("j")
	cmd := h.pressV()
	h.pressV() // wants unviewed again
	if next := h.finish(cmd); next != nil {
		t.Error("nothing more should be sent: the wanted state is the confirmed one")
	}
	if got := h.stateOf(); got != github.FileUnviewed {
		t.Errorf("shown = %q", got)
	}
	if n := len(viewedCalls(h.client)); n != 1 {
		t.Errorf("%d calls, want 1", n)
	}
	if h.lastErr() == nil {
		t.Error("no error shown")
	}
}

func TestFailureMessagesNameTheAction(t *testing.T) {
	h := viewedHarness(t, 100, 24, github.FileViewed)
	h.client.SetError(fake.MethodSetFileViewed, errors.New("boom"))
	h.press("j", "v")
	err := h.lastErr()
	if err == nil || !strings.HasPrefix(err.Error(), "unmark "+firstFile+" as viewed: boom") {
		t.Errorf("error = %v", err)
	}
}

func TestRefreshMidToggleKeepsOptimisticMark(t *testing.T) {
	h := viewedHarness(t, 100, 24)
	h.press("j")
	cmd := h.pressV()
	h.reload(testDetail()) // read before the mutation landed: unviewed
	if got := h.stateOf(); got != github.FileViewed {
		t.Fatalf("refresh dropped the optimistic mark: %q", got)
	}
	h.finish(cmd)
	if got := h.stateOf(); got != github.FileViewed {
		t.Errorf("after success shown = %q", got)
	}
	// Once settled, the next reload shows the server's state again.
	h.v.sequence++ // a reload started after the mutation settled
	d := testDetail()
	d.Files[0].Viewed = github.FileDismissed
	h.reload(d)
	if got := h.stateOf(); got != github.FileDismissed {
		t.Errorf("settled entry still overrides the reload: %q", got)
	}
	if len(h.v.viewedState) != 0 {
		t.Errorf("settled entries kept: %v", h.v.viewedState)
	}
}

func TestFailureAfterRefreshReverts(t *testing.T) {
	h := viewedHarness(t, 100, 24, github.FileUnviewed)
	h.client.SetError(fake.MethodSetFileViewed, errors.New("boom"))
	h.press("j")
	cmd := h.pressV()
	h.reload(unviewedDetail())
	h.finish(cmd)
	if got := h.stateOf(); got != github.FileUnviewed {
		t.Errorf("shown = %q, want reverted", got)
	}
	// The next reload has nothing left to reapply.
	h.reload(unviewedDetail())
	if got := h.stateOf(); got != github.FileUnviewed {
		t.Errorf("shown = %q", got)
	}
}

func TestToggleOnVanishedFileIsDropped(t *testing.T) {
	h := viewedHarness(t, 100, 24)
	h.press("j")
	cmd := h.pressV()
	d := testDetail()
	d.Files = d.Files[1:]
	h.reload(d)
	if len(h.v.viewedState) != 0 {
		t.Errorf("entry for a vanished file kept: %v", h.v.viewedState)
	}
	h.finish(cmd) // must not panic or resurrect anything
	if h.v.viewedCount() != 0 {
		t.Error("a mark appeared on another file")
	}
}

func TestViewedResultForOtherViewIgnored(t *testing.T) {
	tests := []struct {
		name string
		mut  func(m *viewedMessage)
	}{
		{"other number", func(m *viewedMessage) { m.number = 8 }},
		{"other repo", func(m *viewedMessage) { m.repository = github.Repository{Owner: "x", Name: "y"} }},
		{"other view", func(m *viewedMessage) { m.owner = New(nil, testRepository, 7) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := viewedHarness(t, 100, 24)
			h.press("j")
			h.pressV()
			m := viewedMessage{owner: h.v, repository: testRepository, number: 7, path: firstFile, sent: github.FileViewed, err: errors.New("x")}
			tt.mut(&m)
			h.send(m)
			if got := h.stateOf(); got != github.FileViewed {
				t.Errorf("foreign result changed state to %q", got)
			}
		})
	}
}

func TestReloadStartedBeforeSettleKeepsMark(t *testing.T) {
	h := viewedHarness(t, 100, 24, github.FileUnviewed)
	h.press("j")
	cmd := h.pressV()
	h.finish(cmd) // settles at the current load seq
	// A reload that began before the settle finishes with the old data.
	h.reload(unviewedDetail())
	if got := h.stateOf(); got != github.FileViewed {
		t.Fatalf("stale reload overwrote the settled mark: %q", got)
	}
	// A reload started afterwards (a newer seq) shows the server's state.
	h.v.sequence++
	d := unviewedDetail()
	d.Files[0].Viewed = github.FileDismissed
	h.reload(d)
	if got := h.stateOf(); got != github.FileDismissed {
		t.Errorf("later reload not shown: %q", got)
	}
}

func unviewedDetail() github.PullRequestDetail {
	d := testDetail()
	d.Files[0].Viewed = github.FileUnviewed
	return d
}
