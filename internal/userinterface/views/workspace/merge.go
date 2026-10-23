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

func (currentFlow *mergeFlow) same(environment reviewEnvironment, flow *mergeFlow, repository github.Repository, number int) bool {
	return flow == currentFlow && repository == environment.repository && number == environment.number
}

// command handles :merge.
func (currentFlow *mergeFlow) command(arguments string, environment reviewEnvironment) tea.Cmd {
	requested, err := parseMergeArguments(arguments)
	if err != nil {
		return userinterface.Error(err)
	}
	switch {
	case currentFlow.merging:
		return userinterface.Info("a merge is in progress")
	case environment.commentWriting:
		return userinterface.Info(commentSavingText)
	case !environment.loaded:
		return userinterface.Info("pull request not loaded yet")
	case !environment.diffReady:
		return userinterface.Info("diff not loaded")
	}
	currentFlow.sequence++
	flow, sequence := currentFlow, currentFlow.sequence
	return func() tea.Msg {
		status, err := environment.client.GetMergeStatus(context.Background(), environment.repository, environment.number)
		return mergeFreshMessage{flow: flow, repository: environment.repository, number: environment.number, sequence: sequence, requested: requested, status: status, err: err}
	}
}

// update handles the flow's messages. handled is false for any other message.
func (flow *mergeFlow) update(message tea.Msg, environment reviewEnvironment) (command tea.Cmd, handled bool) {
	switch message := message.(type) {
	case mergeFreshMessage:
		if !flow.same(environment, message.flow, message.repository, message.number) || message.sequence != flow.sequence {
			return nil, true
		}
		return flow.confirm(message, environment), true
	case mergeSendMessage:
		if !flow.same(environment, message.flow, message.repository, message.number) {
			return nil, true
		}
		if environment.commentWriting { // a comment write started since the merge did
			return userinterface.Info(commentSavingText), true
		}
		flow.merging = true
		return tea.Batch(userinterface.Info("Merging…"), flow.merge(environment, message.method, message.status)), true
	case mergedMessage:
		if !flow.same(environment, message.flow, message.repository, message.number) {
			return nil, true
		}
		flow.merging = false
		if message.err != nil {
			// Whatever went wrong, the merge may or may not have landed.
			return environment.reload(), true
		}
		return tea.Batch(userinterface.Info(mergedText(environment.number, message.method)), environment.reload()), true
	case mergeRefreshMessage:
		if !flow.same(environment, message.flow, message.repository, message.number) {
			return nil, true
		}
		return environment.reload(), true
	}
	return nil, false
}

// confirm applies the checks that need fresh state, then asks.
func (currentFlow *mergeFlow) confirm(message mergeFreshMessage, environment reviewEnvironment) tea.Cmd {
	if message.err != nil {
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("check merge state: %w", message.err)))
	}
	status := message.status
	// Merge exactly what was reviewed: the commit whose diff is on screen.
	flow := currentFlow
	if environment.diffPending || status.HeadSHA != environment.diffHead {
		refresh := func() tea.Msg {
			return mergeRefreshMessage{flow: flow, repository: environment.repository, number: environment.number}
		}
		return func() tea.Msg {
			return userinterface.ConfirmMessage{Text: headMovedText, Notice: true, OnDismiss: refresh}
		}
	}
	method, err := resolveMethod(message.requested, status)
	if err != nil {
		return userinterface.Error(err)
	}
	send := func() tea.Msg {
		return mergeSendMessage{flow: flow, repository: environment.repository, number: environment.number, method: method, status: status}
	}
	return userinterface.Confirm(mergeQuestion(environment.number, method, status), send)
}

// merge is the command that runs when the user confirms. On failure it also
// returns the error itself, so it shows even if the view was left meanwhile.
func (currentFlow *mergeFlow) merge(environment reviewEnvironment, method github.MergeMethod, status github.MergeStatus) tea.Cmd {
	flow := currentFlow
	in := github.MergeInput{
		Repository: environment.repository, Number: environment.number, PullRequestIdentifier: status.PullRequestIdentifier,
		Method: method, ExpectedHeadObjectIdentifier: status.HeadSHA,
	}
	return func() tea.Msg {
		err := environment.client.MergePullRequest(context.Background(), in)
		result := mergedMessage{flow: flow, repository: environment.repository, number: environment.number, method: method, err: err}
		if err == nil {
			return result
		}
		shown := userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("%w; refreshed to show the current state", err))}
		return tea.BatchMsg{func() tea.Msg { return result }, func() tea.Msg { return shown }}
	}
}
