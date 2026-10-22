package workspace

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// Result messages carry the identity of the request that produced them
// (repo, number and the view's request sequence), because every view in the
// stack receives every non-key message.
type (
	prLoadedMsg struct {
		owner  *View // the workspace that asked; compared, never dereferenced
		repo   github.Repo
		number int
		seq    int
		detail github.PullRequestDetail
		err    error
	}
	diffLoadedMsg struct {
		owner  *View
		repo   github.Repo
		number int
		seq    int
		raw    string
		// parsed is Raw parsed in the command, so Update stays cheap. It is
		// never nil when err is nil, and may be partial when parseErr is set.
		parsed   *diff.Diff
		parseErr error
		err      error // the GetDiff failure
	}
	threadsLoadedMsg struct {
		owner   *View
		repo    github.Repo
		number  int
		seq     int
		threads []github.ReviewThread
		err     error
	}
)

// maxThreadPages bounds the follow-up requests for review threads, so a
// misbehaving server cannot keep the command running forever.
const maxThreadPages = 100

// loadThreads fetches every review thread, page by page. Any failure discards
// the partial result: a half-list would hide threads without saying so.
func loadThreads(ctx context.Context, c github.Client, repo github.Repo, number int) ([]github.ReviewThread, error) {
	var all []github.ReviewThread
	var req github.PageRequest
	for range maxThreadPages {
		p, err := c.ListReviewThreads(ctx, repo, number, req)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Items...)
		if p.Next == "" {
			return all, nil
		}
		req.After = p.Next
	}
	return nil, fmt.Errorf("more than %d pages of review threads", maxThreadPages)
}

// load starts a fresh load of the pull request and its diff.
func (v *View) load() tea.Cmd {
	v.seq++
	v.prPending, v.diffPending, v.thrPending = true, true, true
	v.diff.Loading = true
	v.filePane.SetDiff(v.diff)
	client, repo, number, seq, owner := v.client, v.repo, v.number, v.seq, v

	return tea.Batch(
		func() tea.Msg {
			d, err := client.GetPullRequest(context.Background(), repo, number)
			return prLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, detail: d, err: err}
		},
		func() tea.Msg {
			raw, err := client.GetDiff(context.Background(), repo, number)
			if err != nil {
				return diffLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, err: err}
			}
			// diff.Parse returns a best-effort result with its error, so
			// files before a malformed hunk still show.
			parsed, perr := diff.Parse(raw)
			return diffLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, raw: raw, parsed: parsed, parseErr: perr}
		},
		func() tea.Msg {
			ts, err := loadThreads(context.Background(), client, repo, number)
			return threadsLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, threads: ts, err: err}
		},
	)
}

// mine reports whether a result belongs to this workspace's latest request.
// The owner check matters because results go to every view in the stack: a
// closed workspace's commands may still be running when a new workspace for
// the same pull request starts counting its requests from 1 again.
func (v *View) mine(owner *View, repo github.Repo, number, seq int) bool {
	return owner == v && repo == v.repo && number == v.number && seq == v.seq
}

func (v *View) prLoaded(msg prLoadedMsg) tea.Cmd {
	v.prPending = false
	if msg.err != nil {
		if v.detail == nil {
			v.err, v.notFound = msg.err, errors.Is(msg.err, github.ErrNotFound)
		}
		return ui.Error(termtext.SafeError(msg.err))
	}
	prev := v.selectedPath()
	v.detail, v.err, v.notFound = &msg.detail, nil, false
	top, sel := v.fw.Top(), 0
	for i, f := range msg.detail.Files {
		if prev != "" && f.Path == prev {
			sel = i + 1
		}
	}
	v.fw.SetRows(v.entries())
	v.fw.Restore(sel, top, 0) // a refresh must not scroll the Files list
	v.ov.invalidate()
	v.selectionChanged()
	v.layout()
	return nil
}

func (v *View) threadsLoaded(msg threadsLoadedMsg) tea.Cmd {
	v.thrPending = false
	if msg.err != nil {
		// Keep whatever threads were shown before; the error goes to the status bar.
		return ui.Error(termtext.SafeError(fmt.Errorf("load review threads: %w", msg.err)))
	}
	v.threads = sortThreads(msg.threads)
	v.filePane.SetThreads(v.threads)
	v.ov.invalidate()
	v.layout()
	return nil
}

func (v *View) diffLoaded(msg diffLoadedMsg) tea.Cmd {
	v.diffPending = false
	if msg.err != nil && v.diff.Parsed != nil && !errors.Is(msg.err, github.ErrDiffTooLarge) {
		v.diff.Loading = false // keep the loaded diff; the error goes to the status bar
	} else {
		v.diff = DiffState{Raw: msg.raw, Parsed: msg.parsed, ParseErr: msg.parseErr, Err: msg.err}
	}
	v.filePane.SetDiff(v.diff)
	if msg.parseErr != nil {
		// Files that parsed still show; the problem is reported once here.
		return ui.Error(termtext.SafeError(fmt.Errorf("%s#%d: %w", v.repo, v.number, msg.parseErr)))
	}
	// A diff that is merely too large is shown in the diff pane, not as a
	// transient error.
	if msg.err != nil && !errors.Is(msg.err, github.ErrDiffTooLarge) {
		return ui.Error(termtext.SafeError(msg.err))
	}
	return nil
}
