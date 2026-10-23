package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/picker"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// reviewFlow is the state of submitting a review: the verdict chooser, the
// unsent summary, the request sequence and the in-flight flag. Its methods
// take everything they need from the workspace as a reviewEnv, and result
// messages carry the flow's own pointer as their identity.
type reviewFlow struct {
	chooser    *picker.Model // non-nil while the verdict chooser is open
	sequence   int           // latest review request; older results are stale
	draft      string        // unsent summary, kept until a review is submitted
	submitting bool          // a submission is in flight
}

// reviewEnv is what the flow needs from the workspace at one moment.
type reviewEnvironment struct {
	client      github.Client
	repository  github.Repository
	number      int
	theme       theme.Theme
	edit        EditFunction
	reload      func() tea.Cmd // reloads the workspace
	loaded      bool           // the pull request detail has loaded
	diffHead    string         // head SHA the diff on screen was loaded for
	diffPending bool           // a diff load is in flight
	diffReady   bool           // a diff is loaded and shown
	// commentWriting is set while a comment is being saved.
	commentWriting bool
}

// Results carry the identity of the flow, repo and number that asked. The
// ones that belong to one attempt (pending, edited, fresh) also carry the
// flow's sequence, so starting another review makes older results stale.
type (
	// reviewPendingMsg is the pending review fetched to pre-fill the editor.
	reviewPendingMessage struct {
		flow       *reviewFlow
		repository github.Repository
		number     int
		sequence   int
		event      github.ReviewEvent
		pending    *github.PendingReview
		err        error
	}
	reviewEditedMessage struct {
		flow       *reviewFlow
		repository github.Repository
		number     int
		sequence   int
		event      github.ReviewEvent
		body       string
		err        error
	}
	// reviewFreshMsg is the status fetched just before confirming.
	reviewFreshMessage struct {
		flow       *reviewFlow
		repository github.Repository
		number     int
		sequence   int
		event      github.ReviewEvent
		body       string // the edited summary
		status     github.ReviewStatus
		err        error
	}
	// reviewSendMsg is what the confirmation's "yes" produces; handling it
	// marks the submission as in flight before the call starts.
	reviewSendMessage struct {
		flow       *reviewFlow
		repository github.Repository
		number     int
		event      github.ReviewEvent
		body       string
		status     github.ReviewStatus
	}
	reviewSubmittedMessage struct {
		flow       *reviewFlow
		repository github.Repository
		number     int
		event      github.ReviewEvent
		body       string
		err        error
	}
	// refreshMsg asks the workspace to reload after its head moved.
	refreshMessage struct {
		flow       *reviewFlow
		repository github.Repository
		number     int
	}
)

func (v *View) reviewEnvironment() reviewEnvironment {
	return reviewEnvironment{
		client: v.client, repository: v.repository, number: v.number, theme: v.th,
		edit: v.edit, reload: v.load,
		loaded:      v.detail != nil,
		diffHead:    v.diffHead,
		diffPending: v.diffPending,
		diffReady:   !v.diffPending && v.diff.Parsed != nil && v.diff.Err == nil,

		commentWriting: v.writes > 0,
	}
}

// Command implements userinterface.Commander: :approve, :request-changes, :review,
// :merge, :copy-filename, :edit-comment and :delete-comment.
func (v *View) Command(name, args string) (tea.Cmd, bool) {
	switch name {
	case "copy-filename":
		return v.copyFilename(), true
	case "merge":
		return v.merge.command(args, v.reviewEnvironment()), true
	case "edit-comment":
		return v.editComment(), true
	case "delete-comment":
		return v.deleteComment(), true
	}
	return v.review.command(name, v.reviewEnvironment())
}

func (f *reviewFlow) command(name string, env reviewEnvironment) (tea.Cmd, bool) {
	switch name {
	case "approve":
		return f.start(env, github.ReviewEventApprove), true
	case "request-changes":
		return f.start(env, github.ReviewEventRequestChanges), true
	case "review":
		return f.openChooser(env), true
	}
	return nil, false
}

// mine reports whether a result belongs to this flow's latest attempt.
func (f *reviewFlow) mine(env reviewEnvironment, flow *reviewFlow, repo github.Repository, number, seq int) bool {
	return f.same(env, flow, repo, number) && seq == f.sequence
}

func (f *reviewFlow) same(env reviewEnvironment, flow *reviewFlow, repo github.Repository, number int) bool {
	return flow == f && repo == env.repository && number == env.number
}

// update handles the flow's messages. handled is false for any other message.
func (f *reviewFlow) update(msg tea.Msg, env reviewEnvironment) (cmd tea.Cmd, handled bool) {
	switch msg := msg.(type) {
	case reviewPendingMessage:
		if !f.mine(env, msg.flow, msg.repository, msg.number, msg.sequence) {
			return nil, true
		}
		return f.pendingFetched(msg, env), true
	case reviewEditedMessage:
		if !f.mine(env, msg.flow, msg.repository, msg.number, msg.sequence) {
			return nil, true
		}
		return f.edited(msg, env), true
	case reviewFreshMessage:
		if !f.mine(env, msg.flow, msg.repository, msg.number, msg.sequence) {
			return nil, true
		}
		return f.confirm(msg, env), true
	case reviewSendMessage:
		if !f.same(env, msg.flow, msg.repository, msg.number) {
			return nil, true
		}
		if env.commentWriting { // a comment write started since the review did
			return userinterface.Info(commentSavingText), true
		}
		f.submitting = true
		return tea.Batch(userinterface.Info("Submitting review…"), f.submit(env, msg.event, msg.body, msg.status)), true
	case reviewSubmittedMessage:
		if !f.same(env, msg.flow, msg.repository, msg.number) {
			return nil, true
		}
		return f.submitted(msg, env), true
	case refreshMessage:
		if !f.same(env, msg.flow, msg.repository, msg.number) {
			return nil, true
		}
		return env.reload(), true
	}
	return nil, false
}

// blocked says why a review cannot start now, or returns nil. The diff must
// be on screen: the review's commit is the one the diff shows.
func (f *reviewFlow) blocked(env reviewEnvironment) tea.Cmd {
	switch {
	case f.submitting:
		return userinterface.Info("a review is being submitted")
	case env.commentWriting:
		return userinterface.Info(commentSavingText)
	case !env.loaded:
		return userinterface.Info("pull request not loaded yet")
	case !env.diffReady:
		return userinterface.Info("diff not loaded")
	}
	return nil
}

// openChooser shows the verdict chooser.
func (f *reviewFlow) openChooser(env reviewEnvironment) tea.Cmd {
	if blocked := f.blocked(env); blocked != nil {
		return blocked
	}
	items := make([]string, len(verdicts))
	for i, d := range verdicts {
		items[i] = d.label
	}
	f.chooser = picker.New(env.theme, "Submit review", items)
	return nil
}

// chooserKey feeds a raw key to the open chooser.
func (f *reviewFlow) chooserKey(msg tea.KeyPressMsg, env reviewEnvironment) tea.Cmd {
	res := f.chooser.Update(msg)
	switch res.Outcome {
	case picker.Cancelled:
		f.chooser = nil
	case picker.Chosen:
		f.chooser = nil
		return f.start(env, verdicts[res.Index].event)
	case picker.Open:
	}
	return nil
}

// start fetches the pending review (whose body pre-fills the editor); the
// editor opens when it arrives.
func (f *reviewFlow) start(env reviewEnvironment, event github.ReviewEvent) tea.Cmd {
	if blocked := f.blocked(env); blocked != nil {
		return blocked
	}
	f.sequence++
	flow, seq := f, f.sequence
	return func() tea.Msg {
		st, err := env.client.GetReviewStatus(context.Background(), env.repository, env.number)
		return reviewPendingMessage{flow: flow, repository: env.repository, number: env.number, sequence: seq, event: event, pending: st.Pending, err: err}
	}
}

// pendingFetched opens the editor on the saved draft, or the pending
// review's body.
func (f *reviewFlow) pendingFetched(msg reviewPendingMessage, env reviewEnvironment) tea.Cmd {
	if msg.err != nil {
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("check review state: %w", msg.err)))
	}
	initial := f.draft
	if initial == "" && msg.pending != nil {
		initial = msg.pending.Body
	}
	flow, seq, event := f, f.sequence, msg.event
	return env.edit(initial, func(body string, err error) tea.Msg {
		return reviewEditedMessage{flow: flow, repository: env.repository, number: env.number, sequence: seq, event: event, body: body, err: err}
	})
}

// edited keeps a non-empty body as the draft, then re-queries the state to
// confirm against. An empty body never overwrites an earlier draft, and is
// not stored: it is only sent where that is allowed.
func (f *reviewFlow) edited(msg reviewEditedMessage, env reviewEnvironment) tea.Cmd {
	body := strings.TrimRight(msg.body, " \t\r\n")
	if body != "" {
		f.draft = body
	}
	if msg.err != nil {
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("edit review: %w", msg.err)))
	}
	flow, seq, event := f, f.sequence, msg.event
	return func() tea.Msg {
		st, err := env.client.GetReviewStatus(context.Background(), env.repository, env.number)
		return reviewFreshMessage{flow: flow, repository: env.repository, number: env.number, sequence: seq, event: event, body: body, status: st, err: err}
	}
}

// confirm applies the checks that need fresh state, then asks.
func (f *reviewFlow) confirm(msg reviewFreshMessage, env reviewEnvironment) tea.Cmd {
	if msg.err != nil {
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("check review state: %w", msg.err)))
	}
	st, body := msg.status, msg.body
	// The review applies to the commit whose diff is on screen. If that is
	// not the head now (or the diff is not settled), the user must refresh.
	flow := f
	if env.diffPending || st.HeadSHA != env.diffHead {
		refresh := func() tea.Msg { return refreshMessage{flow: flow, repository: env.repository, number: env.number} }
		return func() tea.Msg {
			return userinterface.ConfirmMessage{Text: headMovedText, Notice: true, OnDismiss: refresh}
		}
	}
	inline := 0
	if st.Pending != nil {
		inline = st.Pending.CommentCount
	}
	if msg.event != github.ReviewEventApprove && strings.TrimSpace(body) == "" && inline == 0 {
		return userinterface.Error(errors.New("empty review, nothing sent (did the editor fail?)"))
	}
	send := func() tea.Msg {
		return reviewSendMessage{flow: flow, repository: env.repository, number: env.number, event: msg.event, body: body, status: st}
	}
	return userinterface.Confirm(reviewQuestion(msg.event, env.repository, env.number, st.Pending), send)
}

// submit is the command that runs when the user confirms. The commit is the
// head the diff on screen was loaded for, which confirm has just checked is
// still the head. On failure it also returns the error itself, so it shows
// even if the view was left meanwhile.
func (f *reviewFlow) submit(env reviewEnvironment, event github.ReviewEvent, body string, st github.ReviewStatus) tea.Cmd {
	flow := f
	in := github.SubmitReviewInput{
		Repository: env.repository, Number: env.number, PullRequestIdentifier: st.PullRequestIdentifier,
		Event: event, Body: body, CommitObjectIdentifier: st.HeadSHA,
	}
	if st.Pending != nil {
		in.PendingReviewIdentifier = st.Pending.Identifier
	}
	return func() tea.Msg {
		err := env.client.SubmitReview(context.Background(), in)
		res := reviewSubmittedMessage{flow: flow, repository: env.repository, number: env.number, event: event, body: body, err: err}
		if err == nil {
			return res
		}
		shown := userinterface.ErrorMessage{Err: submitFailure(err, in.PendingReviewIdentifier != "")}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

func (f *reviewFlow) submitted(msg reviewSubmittedMessage, env reviewEnvironment) tea.Cmd {
	f.submitting = false
	if msg.err != nil {
		// The draft stays for the next attempt. Whatever went wrong, the
		// review may or may not have landed, so show the current state.
		return env.reload()
	}
	f.draft = ""
	return tea.Batch(userinterface.Info(submittedText(msg.event, env.number)), env.reload())
}
