package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/components/picker"
	"github.com/MikePors/strigidae/internal/ui/termtext"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// reviewFlow is the state of submitting a review: the verdict chooser, the
// unsent summary, the request sequence and the in-flight flag. Its methods
// take everything they need from the workspace as a reviewEnv, and result
// messages carry the flow's own pointer as their identity.
type reviewFlow struct {
	chooser    *picker.Model // non-nil while the verdict chooser is open
	seq        int           // latest review request; older results are stale
	draft      string        // unsent summary, kept until a review is submitted
	submitting bool          // a submission is in flight
}

// reviewEnv is what the flow needs from the workspace at one moment.
type reviewEnv struct {
	client      github.Client
	repo        github.Repo
	number      int
	theme       theme.Theme
	edit        EditFunc
	reload      func() tea.Cmd // reloads the workspace
	loaded      bool           // the pull request detail has loaded
	diffHead    string         // head SHA the diff on screen was loaded for
	diffPending bool           // a diff load is in flight
	diffReady   bool           // a diff is loaded and shown
}

// Results carry the identity of the flow, repo and number that asked. The
// ones that belong to one attempt (pending, edited, fresh) also carry the
// flow's sequence, so starting another review makes older results stale.
type (
	// reviewPendingMsg is the pending review fetched to pre-fill the editor.
	reviewPendingMsg struct {
		flow    *reviewFlow
		repo    github.Repo
		number  int
		seq     int
		event   github.ReviewEvent
		pending *github.PendingReview
		err     error
	}
	reviewEditedMsg struct {
		flow   *reviewFlow
		repo   github.Repo
		number int
		seq    int
		event  github.ReviewEvent
		body   string
		err    error
	}
	// reviewFreshMsg is the status fetched just before confirming.
	reviewFreshMsg struct {
		flow   *reviewFlow
		repo   github.Repo
		number int
		seq    int
		event  github.ReviewEvent
		body   string // the edited summary
		status github.ReviewStatus
		err    error
	}
	// reviewSendMsg is what the confirmation's "yes" produces; handling it
	// marks the submission as in flight before the call starts.
	reviewSendMsg struct {
		flow   *reviewFlow
		repo   github.Repo
		number int
		event  github.ReviewEvent
		body   string
		status github.ReviewStatus
	}
	reviewSubmittedMsg struct {
		flow   *reviewFlow
		repo   github.Repo
		number int
		event  github.ReviewEvent
		body   string
		err    error
	}
	// refreshMsg asks the workspace to reload after its head moved.
	refreshMsg struct {
		flow   *reviewFlow
		repo   github.Repo
		number int
	}
)

func (v *View) reviewEnv() reviewEnv {
	return reviewEnv{
		client: v.client, repo: v.repo, number: v.number, theme: v.th,
		edit: v.edit, reload: v.load,
		loaded:      v.detail != nil,
		diffHead:    v.diffHead,
		diffPending: v.diffPending,
		diffReady:   !v.diffPending && v.diff.Parsed != nil && v.diff.Err == nil,
	}
}

// Command implements ui.Commander: :approve, :request-changes, :review and
// :copy-filename.
func (v *View) Command(name, _ string) (tea.Cmd, bool) {
	if name == "copy-filename" {
		return v.copyFilename(), true
	}
	return v.review.command(name, v.reviewEnv())
}

func (f *reviewFlow) command(name string, env reviewEnv) (tea.Cmd, bool) {
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
func (f *reviewFlow) mine(env reviewEnv, flow *reviewFlow, repo github.Repo, number, seq int) bool {
	return f.same(env, flow, repo, number) && seq == f.seq
}

func (f *reviewFlow) same(env reviewEnv, flow *reviewFlow, repo github.Repo, number int) bool {
	return flow == f && repo == env.repo && number == env.number
}

// update handles the flow's messages. handled is false for any other message.
func (f *reviewFlow) update(msg tea.Msg, env reviewEnv) (cmd tea.Cmd, handled bool) {
	switch msg := msg.(type) {
	case reviewPendingMsg:
		if !f.mine(env, msg.flow, msg.repo, msg.number, msg.seq) {
			return nil, true
		}
		return f.pendingFetched(msg, env), true
	case reviewEditedMsg:
		if !f.mine(env, msg.flow, msg.repo, msg.number, msg.seq) {
			return nil, true
		}
		return f.edited(msg, env), true
	case reviewFreshMsg:
		if !f.mine(env, msg.flow, msg.repo, msg.number, msg.seq) {
			return nil, true
		}
		return f.confirm(msg, env), true
	case reviewSendMsg:
		if !f.same(env, msg.flow, msg.repo, msg.number) {
			return nil, true
		}
		f.submitting = true
		return tea.Batch(ui.Info("Submitting review…"), f.submit(env, msg.event, msg.body, msg.status)), true
	case reviewSubmittedMsg:
		if !f.same(env, msg.flow, msg.repo, msg.number) {
			return nil, true
		}
		return f.submitted(msg, env), true
	case refreshMsg:
		if !f.same(env, msg.flow, msg.repo, msg.number) {
			return nil, true
		}
		return env.reload(), true
	}
	return nil, false
}

// blocked says why a review cannot start now, or returns nil. The diff must
// be on screen: the review's commit is the one the diff shows.
func (f *reviewFlow) blocked(env reviewEnv) tea.Cmd {
	switch {
	case f.submitting:
		return ui.Info("a review is being submitted")
	case !env.loaded:
		return ui.Info("pull request not loaded yet")
	case !env.diffReady:
		return ui.Info("diff not loaded")
	}
	return nil
}

// openChooser shows the verdict chooser.
func (f *reviewFlow) openChooser(env reviewEnv) tea.Cmd {
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
func (f *reviewFlow) chooserKey(msg tea.KeyPressMsg, env reviewEnv) tea.Cmd {
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
func (f *reviewFlow) start(env reviewEnv, event github.ReviewEvent) tea.Cmd {
	if blocked := f.blocked(env); blocked != nil {
		return blocked
	}
	f.seq++
	flow, seq := f, f.seq
	return func() tea.Msg {
		st, err := env.client.GetReviewStatus(context.Background(), env.repo, env.number)
		return reviewPendingMsg{flow: flow, repo: env.repo, number: env.number, seq: seq, event: event, pending: st.Pending, err: err}
	}
}

// pendingFetched opens the editor on the saved draft, or the pending
// review's body.
func (f *reviewFlow) pendingFetched(msg reviewPendingMsg, env reviewEnv) tea.Cmd {
	if msg.err != nil {
		return ui.Error(termtext.SafeError(fmt.Errorf("check review state: %w", msg.err)))
	}
	initial := f.draft
	if initial == "" && msg.pending != nil {
		initial = msg.pending.Body
	}
	flow, seq, event := f, f.seq, msg.event
	return env.edit(initial, func(body string, err error) tea.Msg {
		return reviewEditedMsg{flow: flow, repo: env.repo, number: env.number, seq: seq, event: event, body: body, err: err}
	})
}

// edited keeps a non-empty body as the draft, then re-queries the state to
// confirm against. An empty body never overwrites an earlier draft, and is
// not stored: it is only sent where that is allowed.
func (f *reviewFlow) edited(msg reviewEditedMsg, env reviewEnv) tea.Cmd {
	body := strings.TrimRight(msg.body, " \t\r\n")
	if body != "" {
		f.draft = body
	}
	if msg.err != nil {
		return ui.Error(termtext.SafeError(fmt.Errorf("edit review: %w", msg.err)))
	}
	flow, seq, event := f, f.seq, msg.event
	return func() tea.Msg {
		st, err := env.client.GetReviewStatus(context.Background(), env.repo, env.number)
		return reviewFreshMsg{flow: flow, repo: env.repo, number: env.number, seq: seq, event: event, body: body, status: st, err: err}
	}
}

// confirm applies the checks that need fresh state, then asks.
func (f *reviewFlow) confirm(msg reviewFreshMsg, env reviewEnv) tea.Cmd {
	if msg.err != nil {
		return ui.Error(termtext.SafeError(fmt.Errorf("check review state: %w", msg.err)))
	}
	st, body := msg.status, msg.body
	// The review applies to the commit whose diff is on screen. If that is
	// not the head now (or the diff is not settled), the user must refresh.
	flow := f
	if env.diffPending || st.HeadSHA != env.diffHead {
		refresh := func() tea.Msg { return refreshMsg{flow: flow, repo: env.repo, number: env.number} }
		return func() tea.Msg {
			return ui.ConfirmMsg{Text: headMovedText, Notice: true, OnDismiss: refresh}
		}
	}
	inline := 0
	if st.Pending != nil {
		inline = st.Pending.CommentCount
	}
	if msg.event != github.ReviewEventApprove && strings.TrimSpace(body) == "" && inline == 0 {
		return ui.Error(errors.New("empty review, nothing sent (did the editor fail?)"))
	}
	send := func() tea.Msg {
		return reviewSendMsg{flow: flow, repo: env.repo, number: env.number, event: msg.event, body: body, status: st}
	}
	return ui.Confirm(reviewQuestion(msg.event, env.repo, env.number, st.Pending), send)
}

// submit is the command that runs when the user confirms. The commit is the
// head the diff on screen was loaded for, which confirm has just checked is
// still the head. On failure it also returns the error itself, so it shows
// even if the view was left meanwhile.
func (f *reviewFlow) submit(env reviewEnv, event github.ReviewEvent, body string, st github.ReviewStatus) tea.Cmd {
	flow := f
	in := github.SubmitReviewInput{
		Repo: env.repo, Number: env.number, PullRequestID: st.PullRequestID,
		Event: event, Body: body, CommitOID: st.HeadSHA,
	}
	if st.Pending != nil {
		in.PendingReviewID = st.Pending.ID
	}
	return func() tea.Msg {
		err := env.client.SubmitReview(context.Background(), in)
		res := reviewSubmittedMsg{flow: flow, repo: env.repo, number: env.number, event: event, body: body, err: err}
		if err == nil {
			return res
		}
		shown := ui.ErrorMsg{Err: submitFailure(err, in.PendingReviewID != "")}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

func (f *reviewFlow) submitted(msg reviewSubmittedMsg, env reviewEnv) tea.Cmd {
	f.submitting = false
	if msg.err != nil {
		// The draft stays for the next attempt. Whatever went wrong, the
		// review may or may not have landed, so show the current state.
		return env.reload()
	}
	f.draft = ""
	return tea.Batch(ui.Info(submittedText(msg.event, env.number)), env.reload())
}
