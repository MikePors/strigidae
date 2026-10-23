package workspace

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// Result messages carry the identity of the request that produced them
// (repo, number and the view's request sequence), because every view in the
// stack receives every non-key message.
type (
	prLoadedMsg struct {
		owner  *View // the workspace that asked; compared, never dereferenced
		repo   github.Repository
		number int
		seq    int
		detail github.PullRequestDetail
		err    error
		// thenDiff makes the result start the diff load. The diff is fetched
		// after the detail so that the head recorded for it can only be older
		// than the diff, never newer.
		thenDiff bool
	}
	diffLoadedMsg struct {
		owner  *View
		repo   github.Repository
		number int
		seq    int
		// head is the detail's head SHA this diff followed; empty when the
		// detail failed to load.
		head string
		raw  string
		// parsed is Raw parsed in the command, so Update stays cheap. It is
		// never nil when err is nil, and may be partial when parseErr is set.
		parsed   *diff.Diff
		parseErr error
		err      error // the GetDiff failure
	}
	threadsLoadedMsg struct {
		owner   *View
		repo    github.Repository
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
func loadThreads(ctx context.Context, c github.Client, repo github.Repository, number int) ([]github.ReviewThread, error) {
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
	v.diffPending = true
	v.diff.Loading = true
	v.filePane.SetDiff(v.diff)

	return tea.Batch(v.fetchPR(true), v.reloadThreads())
}

// fetchDiff loads the diff once the detail has landed.
func (v *View) fetchDiff(msg prLoadedMsg) tea.Cmd {
	client, repo, number, seq, owner := v.client, v.repo, v.number, msg.seq, v
	head := ""
	if msg.err == nil {
		head = msg.detail.HeadSHA
	}
	return func() tea.Msg {
		raw, err := client.GetDiff(context.Background(), repo, number)
		if err != nil {
			return diffLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, err: err}
		}
		// diff.Parse returns a best-effort result with its error, so
		// files before a malformed hunk still show.
		parsed, perr := diff.Parse(raw)
		return diffLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, head: head, raw: raw, parsed: parsed, parseErr: perr}
	}
}

// reloadPR loads the pull request again under the current request sequence;
// after posting a comment it refreshes just the conversation.
func (v *View) reloadPR() tea.Cmd { return v.fetchPR(false) }

func (v *View) fetchPR(thenDiff bool) tea.Cmd {
	v.prPending = true
	v.ov.invalidate()
	client, repo, number, seq, owner := v.client, v.repo, v.number, v.seq, v
	return func() tea.Msg {
		d, err := client.GetPullRequest(context.Background(), repo, number)
		return prLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, detail: d, err: err, thenDiff: thenDiff}
	}
}

// reloadThreads loads the review threads again under the current request
// sequence; after posting a reply it refreshes just the threads.
func (v *View) reloadThreads() tea.Cmd {
	v.thrPending, v.thrErr = true, nil
	client, repo, number, seq, owner := v.client, v.repo, v.number, v.seq, v
	return func() tea.Msg {
		ts, err := loadThreads(context.Background(), client, repo, number)
		return threadsLoadedMsg{owner: owner, repo: repo, number: number, seq: seq, threads: ts, err: err}
	}
}

// mine reports whether a result belongs to this workspace's latest request.
// The owner check matters because results go to every view in the stack: a
// closed workspace's commands may still be running when a new workspace for
// the same pull request starts counting its requests from 1 again.
func (v *View) mine(owner *View, repo github.Repository, number, seq int) bool {
	return owner == v && repo == v.repo && number == v.number && seq == v.seq
}

func (v *View) prLoaded(msg prLoadedMsg) tea.Cmd {
	v.prPending = false
	if msg.err != nil {
		if v.detail == nil {
			v.err, v.notFound = msg.err, errors.Is(msg.err, github.ErrNotFound)
		}
		return userinterface.Error(terminaltext.SafeError(msg.err))
	}
	prev := v.selectedPath()
	v.detail, v.err, v.notFound = &msg.detail, nil, false
	v.reapplyViewed(msg.seq)
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
		// Keep whatever threads were shown before; the error goes to the status
		// bar, and to the Overview while there is nothing else to show.
		v.thrErr = msg.err
		v.ov.invalidate()
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("load review threads: %w", msg.err)))
	}
	v.threads, v.thrLoaded = sortThreads(msg.threads), true
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
		v.diffHead = msg.head
	}
	v.filePane.SetDiff(v.diff)
	if msg.parseErr != nil {
		// Files that parsed still show; the problem is reported once here.
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("%s#%d: %w", v.repo, v.number, msg.parseErr)))
	}
	// A diff that is merely too large is shown in the diff pane, not as a
	// transient error.
	if msg.err != nil && !errors.Is(msg.err, github.ErrDiffTooLarge) {
		return userinterface.Error(terminaltext.SafeError(msg.err))
	}
	return nil
}
