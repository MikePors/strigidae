package workspace

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// viewedToggle is the client-side state of one file's viewed mark while a
// toggle is in flight or not yet reflected by a reload. Mutations for a path
// are sent one at a time, so the server cannot see them out of order.
type viewedToggle struct {
	inflight  bool
	want      github.ViewedState // latest state the user asked for; shown at once
	confirmed github.ViewedState // last state known to be on the server
	settledAt int                // load seq current when the toggle settled
}

// viewedMsg is the result of one mutation. It is matched on owner, repo,
// number and path, not on the load sequence: a reload does not make the
// result irrelevant.
type viewedMessage struct {
	owner      *View
	repository github.Repository
	number     int
	path       string
	sent       github.ViewedState // state the mutation asked for
	err        error
}

// viewedCount returns how many files are marked viewed. Dismissed files
// (changed since they were viewed) do not count.
func (v *View) viewedCount() int {
	n := 0
	for _, f := range v.files() {
		if f.Viewed == github.FileViewed {
			n++
		}
	}
	return n
}

// filesTitle is the Files pane title, with review progress once files exist.
func (v *View) filesTitle() string {
	if n := len(v.files()); n > 0 {
		return fmt.Sprintf("Files %d/%d viewed", v.viewedCount(), n)
	}
	return "Files"
}

// toggleViewed flips the viewed mark of the selected file at once. A viewed
// file becomes unviewed; an unviewed or dismissed one becomes viewed. The
// mutation is sent now if none is in flight for the file, else when the
// running one finishes.
func (v *View) toggleViewed() tea.Cmd {
	s := v.sel()
	if s == 0 || s > len(v.files()) {
		return nil
	}
	if v.detail.Identifier == "" {
		return userinterface.Error(errors.New("mark file as viewed: pull request ID unknown"))
	}
	f := &v.detail.Files[s-1]
	t := v.viewedState[f.Path]
	if t == nil {
		t = &viewedToggle{confirmed: f.Viewed}
		v.viewedState[f.Path] = t
	}
	t.want = github.FileViewed
	if f.Viewed == github.FileViewed {
		t.want = github.FileUnviewed
	}
	f.Viewed = t.want
	if t.inflight {
		return nil
	}
	t.inflight = true
	return v.sendViewed(f.Path, t.want)
}

// sendViewed is the command that runs one mutation. On failure it also
// returns the error itself, so it shows even if the view was left meanwhile.
func (v *View) sendViewed(path string, state github.ViewedState) tea.Cmd {
	client, repo, number, owner, prID := v.client, v.repository, v.number, v, v.detail.Identifier
	return func() tea.Msg {
		err := client.SetFileViewed(context.Background(), prID, path, state == github.FileViewed)
		res := viewedMessage{owner: owner, repository: repo, number: number, path: path, sent: state, err: err}
		if err == nil {
			return res
		}
		verb := "mark"
		if state != github.FileViewed {
			verb = "unmark"
		}
		shown := userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("%s %s as viewed: %w", verb, terminaltext.Clean(path), err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

func (v *View) viewedMine(msg viewedMessage) bool {
	return msg.owner == v && msg.repository == v.repository && msg.number == v.number
}

// viewedDone records a mutation's outcome and sends the next wanted state, if
// the user toggled again meanwhile.
func (v *View) viewedDone(msg viewedMessage) tea.Cmd {
	t := v.viewedState[msg.path]
	if t == nil {
		return nil
	}
	if msg.err == nil {
		t.confirmed = msg.sent
	} else {
		t.want = t.confirmed
		v.setViewed(msg.path, t.want)
	}
	if t.want != msg.sent && t.want != t.confirmed {
		return v.sendViewed(msg.path, t.want)
	}
	t.inflight, t.settledAt = false, v.sequence
	return nil
}

func (v *View) setViewed(path string, state github.ViewedState) {
	if v.detail == nil {
		return
	}
	for i := range v.detail.Files {
		if v.detail.Files[i].Path == path {
			v.detail.Files[i].Viewed = state
		}
	}
}

// reapplyViewed runs after the detail is replaced by a reload. Files with a
// toggle in flight or unconfirmed keep their wanted mark, because the reload
// may have read before the mutation landed; settled entries are dropped so
// the server's state shows (once the reload started after they settled), and so
// are entries for files that vanished.
func (v *View) reapplyViewed(seq int) {
	present := map[string]bool{}
	for _, f := range v.files() {
		present[f.Path] = true
	}
	for path, t := range v.viewedState {
		settled := !t.inflight && t.want == t.confirmed
		switch {
		case !present[path], settled && seq > t.settledAt:
			delete(v.viewedState, path)
		default:
			// A settled entry whose reload began before it settled may hold
			// older server data; t.want equals t.confirmed then.
			v.setViewed(path, t.want)
		}
	}
}
