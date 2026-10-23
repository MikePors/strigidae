package workspace

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// mergeFlow is the state of merging the pull request. It takes what it needs
// from the workspace as a reviewEnv, and result messages carry the flow's own
// pointer as their identity.
type mergeFlow struct {
	sequence int  // latest merge request; older results are stale
	merging  bool // a merge is in flight
}

type (
	// mergeFreshMsg is the status fetched just before confirming.
	mergeFreshMessage struct {
		flow       *mergeFlow
		repository github.Repository
		number     int
		sequence   int
		requested  github.MergeMethod // "" for the default
		status     github.MergeStatus
		err        error
	}
	// mergeSendMsg is what the confirmation's "yes" produces; handling it
	// marks the merge as in flight before the call starts.
	mergeSendMessage struct {
		flow       *mergeFlow
		repository github.Repository
		number     int
		method     github.MergeMethod
		status     github.MergeStatus
	}
	mergedMessage struct {
		flow       *mergeFlow
		repository github.Repository
		number     int
		method     github.MergeMethod
		err        error
	}
	// mergeRefreshMsg asks the workspace to reload after its head moved.
	mergeRefreshMessage struct {
		flow       *mergeFlow
		repository github.Repository
		number     int
	}
)

func (f *mergeFlow) same(env reviewEnvironment, flow *mergeFlow, repo github.Repository, number int) bool {
	return flow == f && repo == env.repository && number == env.number
}

// command handles :merge.
func (f *mergeFlow) command(args string, env reviewEnvironment) tea.Cmd {
	requested, err := parseMergeArguments(args)
	if err != nil {
		return userinterface.Error(err)
	}
	switch {
	case f.merging:
		return userinterface.Info("a merge is in progress")
	case env.commentWriting:
		return userinterface.Info(commentSavingText)
	case !env.loaded:
		return userinterface.Info("pull request not loaded yet")
	case !env.diffReady:
		return userinterface.Info("diff not loaded")
	}
	f.sequence++
	flow, seq := f, f.sequence
	return func() tea.Msg {
		st, err := env.client.GetMergeStatus(context.Background(), env.repository, env.number)
		return mergeFreshMessage{flow: flow, repository: env.repository, number: env.number, sequence: seq, requested: requested, status: st, err: err}
	}
}

// update handles the flow's messages. handled is false for any other message.
func (f *mergeFlow) update(msg tea.Msg, env reviewEnvironment) (cmd tea.Cmd, handled bool) {
	switch msg := msg.(type) {
	case mergeFreshMessage:
		if !f.same(env, msg.flow, msg.repository, msg.number) || msg.sequence != f.sequence {
			return nil, true
		}
		return f.confirm(msg, env), true
	case mergeSendMessage:
		if !f.same(env, msg.flow, msg.repository, msg.number) {
			return nil, true
		}
		if env.commentWriting { // a comment write started since the merge did
			return userinterface.Info(commentSavingText), true
		}
		f.merging = true
		return tea.Batch(userinterface.Info("Merging…"), f.merge(env, msg.method, msg.status)), true
	case mergedMessage:
		if !f.same(env, msg.flow, msg.repository, msg.number) {
			return nil, true
		}
		f.merging = false
		if msg.err != nil {
			// Whatever went wrong, the merge may or may not have landed.
			return env.reload(), true
		}
		return tea.Batch(userinterface.Info(mergedText(env.number, msg.method)), env.reload()), true
	case mergeRefreshMessage:
		if !f.same(env, msg.flow, msg.repository, msg.number) {
			return nil, true
		}
		return env.reload(), true
	}
	return nil, false
}

// confirm applies the checks that need fresh state, then asks.
func (f *mergeFlow) confirm(msg mergeFreshMessage, env reviewEnvironment) tea.Cmd {
	if msg.err != nil {
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("check merge state: %w", msg.err)))
	}
	st := msg.status
	// Merge exactly what was reviewed: the commit whose diff is on screen.
	flow := f
	if env.diffPending || st.HeadSHA != env.diffHead {
		refresh := func() tea.Msg { return mergeRefreshMessage{flow: flow, repository: env.repository, number: env.number} }
		return func() tea.Msg {
			return userinterface.ConfirmMessage{Text: headMovedText, Notice: true, OnDismiss: refresh}
		}
	}
	method, err := resolveMethod(msg.requested, st)
	if err != nil {
		return userinterface.Error(err)
	}
	send := func() tea.Msg {
		return mergeSendMessage{flow: flow, repository: env.repository, number: env.number, method: method, status: st}
	}
	return userinterface.Confirm(mergeQuestion(env.number, method, st), send)
}

// merge is the command that runs when the user confirms. On failure it also
// returns the error itself, so it shows even if the view was left meanwhile.
func (f *mergeFlow) merge(env reviewEnvironment, method github.MergeMethod, st github.MergeStatus) tea.Cmd {
	flow := f
	in := github.MergeInput{
		Repository: env.repository, Number: env.number, PullRequestIdentifier: st.PullRequestIdentifier,
		Method: method, ExpectedHeadObjectIdentifier: st.HeadSHA,
	}
	return func() tea.Msg {
		err := env.client.MergePullRequest(context.Background(), in)
		res := mergedMessage{flow: flow, repository: env.repository, number: env.number, method: method, err: err}
		if err == nil {
			return res
		}
		shown := userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("%w; refreshed to show the current state", err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}
