package workspace

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// viewedMsg is the result of a mark/unmark mutation. It carries the load
// identity plus the toggle it belongs to, so a failure reverts only a mark
// that still holds what this toggle set.
type viewedMsg struct {
	owner  *View
	repo   github.Repo
	number int
	seq    int
	path   string
	from   github.ViewedState // state before the toggle
	to     github.ViewedState // state the toggle set
	err    error
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

// toggleViewed flips the viewed mark of the selected file at once and sends
// the change to GitHub in the background. A file that is viewed becomes
// unviewed; an unviewed or dismissed one becomes viewed.
func (v *View) toggleViewed() tea.Cmd {
	s := v.sel()
	if s == 0 || s > len(v.files()) {
		return nil
	}
	if v.detail.ID == "" {
		return ui.Error(errors.New("mark file as viewed: pull request ID unknown"))
	}
	f := &v.detail.Files[s-1]
	from, to := f.Viewed, github.FileViewed
	if from == github.FileViewed {
		to = github.FileUnviewed
	}
	f.Viewed = to

	client, repo, number, seq, owner := v.client, v.repo, v.number, v.seq, v
	prID, path := v.detail.ID, f.Path
	return func() tea.Msg {
		err := client.SetFileViewed(context.Background(), prID, path, to == github.FileViewed)
		res := viewedMsg{owner: owner, repo: repo, number: number, seq: seq, path: path, from: from, to: to, err: err}
		if err == nil {
			return res
		}
		// The error is also sent on its own so it shows even if the view
		// was left or refreshed meanwhile.
		shown := ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("mark %s as viewed: %w", termtext.Clean(path), err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

// viewedDone reverts a failed toggle, unless the file has changed again since.
func (v *View) viewedDone(msg viewedMsg) {
	if msg.err == nil || v.detail == nil {
		return
	}
	for i := range v.detail.Files {
		f := &v.detail.Files[i]
		if f.Path == msg.path && f.Viewed == msg.to {
			f.Viewed = msg.from
		}
	}
}
