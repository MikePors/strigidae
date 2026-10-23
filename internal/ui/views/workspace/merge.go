package workspace

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// mergeFlow is the state of merging the pull request. It takes what it needs
// from the workspace as a reviewEnv, and result messages carry the flow's own
// pointer as their identity.
type mergeFlow struct {
	seq     int  // latest merge request; older results are stale
	merging bool // a merge is in flight
}

type (
	// mergeFreshMsg is the status fetched just before confirming.
	mergeFreshMsg struct {
		flow      *mergeFlow
		repo      github.Repo
		number    int
		seq       int
		requested github.MergeMethod // "" for the default
		status    github.MergeStatus
		err       error
	}
	// mergeSendMsg is what the confirmation's "yes" produces; handling it
	// marks the merge as in flight before the call starts.
	mergeSendMsg struct {
		flow   *mergeFlow
		repo   github.Repo
		number int
		method github.MergeMethod
		status github.MergeStatus
	}
	mergedMsg struct {
		flow   *mergeFlow
		repo   github.Repo
		number int
		method github.MergeMethod
		err    error
	}
	// mergeRefreshMsg asks the workspace to reload after its head moved.
	mergeRefreshMsg struct {
		flow   *mergeFlow
		repo   github.Repo
		number int
	}
)

func (f *mergeFlow) same(env reviewEnv, flow *mergeFlow, repo github.Repo, number int) bool {
	return flow == f && repo == env.repo && number == env.number
}

// command handles :merge.
func (f *mergeFlow) command(args string, env reviewEnv) tea.Cmd {
	requested, err := parseMergeArgs(args)
	if err != nil {
		return ui.Error(err)
	}
	switch {
	case f.merging:
		return ui.Info("a merge is in progress")
	case env.commentWriting:
		return ui.Info(commentSavingText)
	case !env.loaded:
		return ui.Info("pull request not loaded yet")
	case !env.diffReady:
		return ui.Info("diff not loaded")
	}
	f.seq++
	flow, seq := f, f.seq
	return func() tea.Msg {
		st, err := env.client.GetMergeStatus(context.Background(), env.repo, env.number)
		return mergeFreshMsg{flow: flow, repo: env.repo, number: env.number, seq: seq, requested: requested, status: st, err: err}
	}
}

// update handles the flow's messages. handled is false for any other message.
func (f *mergeFlow) update(msg tea.Msg, env reviewEnv) (cmd tea.Cmd, handled bool) {
	switch msg := msg.(type) {
	case mergeFreshMsg:
		if !f.same(env, msg.flow, msg.repo, msg.number) || msg.seq != f.seq {
			return nil, true
		}
		return f.confirm(msg, env), true
	case mergeSendMsg:
		if !f.same(env, msg.flow, msg.repo, msg.number) {
			return nil, true
		}
		if env.commentWriting { // a comment write started since the merge did
			return ui.Info(commentSavingText), true
		}
		f.merging = true
		return tea.Batch(ui.Info("Merging…"), f.merge(env, msg.method, msg.status)), true
	case mergedMsg:
		if !f.same(env, msg.flow, msg.repo, msg.number) {
			return nil, true
		}
		f.merging = false
		if msg.err != nil {
			// Whatever went wrong, the merge may or may not have landed.
			return env.reload(), true
		}
		return tea.Batch(ui.Info(mergedText(env.number, msg.method)), env.reload()), true
	case mergeRefreshMsg:
		if !f.same(env, msg.flow, msg.repo, msg.number) {
			return nil, true
		}
		return env.reload(), true
	}
	return nil, false
}

// confirm applies the checks that need fresh state, then asks.
func (f *mergeFlow) confirm(msg mergeFreshMsg, env reviewEnv) tea.Cmd {
	if msg.err != nil {
		return ui.Error(termtext.SafeError(fmt.Errorf("check merge state: %w", msg.err)))
	}
	st := msg.status
	// Merge exactly what was reviewed: the commit whose diff is on screen.
	flow := f
	if env.diffPending || st.HeadSHA != env.diffHead {
		refresh := func() tea.Msg { return mergeRefreshMsg{flow: flow, repo: env.repo, number: env.number} }
		return func() tea.Msg {
			return ui.ConfirmMsg{Text: headMovedText, Notice: true, OnDismiss: refresh}
		}
	}
	method, err := resolveMethod(msg.requested, st)
	if err != nil {
		return ui.Error(err)
	}
	send := func() tea.Msg {
		return mergeSendMsg{flow: flow, repo: env.repo, number: env.number, method: method, status: st}
	}
	return ui.Confirm(mergeQuestion(env.number, method, st), send)
}

// merge is the command that runs when the user confirms. On failure it also
// returns the error itself, so it shows even if the view was left meanwhile.
func (f *mergeFlow) merge(env reviewEnv, method github.MergeMethod, st github.MergeStatus) tea.Cmd {
	flow := f
	in := github.MergeInput{
		Repo: env.repo, Number: env.number, PullRequestID: st.PullRequestID,
		Method: method, ExpectedHeadOID: st.HeadSHA,
	}
	return func() tea.Msg {
		err := env.client.MergePullRequest(context.Background(), in)
		res := mergedMsg{flow: flow, repo: env.repo, number: env.number, method: method, err: err}
		if err == nil {
			return res
		}
		shown := ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("%w; refreshed to show the current state", err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}
