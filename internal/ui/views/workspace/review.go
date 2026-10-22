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
)

// verdicts are the chooser's rows, in order.
var verdicts = []struct {
	label string
	event github.ReviewEvent
}{
	{"Approve", github.ReviewEventApprove},
	{"Request changes", github.ReviewEventRequestChanges},
	{"Comment", github.ReviewEventComment},
}

// reviewStage says what a status result is for.
type reviewStage int

const (
	// stageEdit: the status only supplies the pending review's body, so the
	// editor can open pre-filled.
	stageEdit reviewStage = iota
	// stageConfirm: the status is the fresh state shown in the confirmation.
	stageConfirm
)

// Results of submitting a review carry the same identity as load results. The
// status and edit results also carry the review sequence, so starting another
// review makes older ones stale.
type (
	reviewStatusMsg struct {
		owner  *View
		repo   github.Repo
		number int
		seq    int
		stage  reviewStage
		event  github.ReviewEvent
		status github.ReviewStatus
		err    error
	}
	reviewEditedMsg struct {
		owner  *View
		repo   github.Repo
		number int
		seq    int
		event  github.ReviewEvent
		body   string
		err    error
	}
	reviewSubmittedMsg struct {
		owner  *View
		repo   github.Repo
		number int
		event  github.ReviewEvent
		body   string
		err    error
	}
	// refreshMsg asks the workspace to reload after its head moved.
	refreshMsg struct {
		owner  *View
		repo   github.Repo
		number int
	}
)

func (v *View) reviewMine(owner *View, repo github.Repo, number, seq int) bool {
	return owner == v && repo == v.repo && number == v.number && seq == v.rseq
}

func (v *View) sameReview(owner *View, repo github.Repo, number int) bool {
	return owner == v && repo == v.repo && number == v.number
}

// Command implements ui.Commander: :approve, :request-changes and :review.
func (v *View) Command(name, _ string) (tea.Cmd, bool) {
	switch name {
	case "approve":
		return v.startReview(github.ReviewEventApprove), true
	case "request-changes":
		return v.startReview(github.ReviewEventRequestChanges), true
	case "review":
		return v.openChooser(), true
	}
	return nil, false
}

// openChooser shows the verdict chooser.
func (v *View) openChooser() tea.Cmd {
	if v.detail == nil {
		return ui.Info("pull request not loaded yet")
	}
	items := make([]string, len(verdicts))
	for i, d := range verdicts {
		items[i] = d.label
	}
	v.chooser = picker.New(v.th, "Submit review", items)
	return nil
}

// chooserKey feeds a raw key to the open chooser.
func (v *View) chooserKey(msg tea.KeyPressMsg) tea.Cmd {
	res := v.chooser.Update(msg)
	switch res.Outcome {
	case picker.Cancelled:
		v.chooser = nil
	case picker.Chosen:
		v.chooser = nil
		return v.startReview(verdicts[res.Index].event)
	case picker.Open:
	}
	return nil
}

// startReview fetches the pending review (whose body pre-fills the editor)
// and then opens the editor.
func (v *View) startReview(event github.ReviewEvent) tea.Cmd {
	if v.detail == nil {
		return ui.Info("pull request not loaded yet")
	}
	v.rseq++
	return v.fetchStatus(stageEdit, event)
}

func (v *View) fetchStatus(stage reviewStage, event github.ReviewEvent) tea.Cmd {
	client, repo, number, seq, owner := v.client, v.repo, v.number, v.rseq, v
	return func() tea.Msg {
		st, err := client.GetReviewStatus(context.Background(), repo, number)
		return reviewStatusMsg{owner: owner, repo: repo, number: number, seq: seq, stage: stage, event: event, status: st, err: err}
	}
}

func (v *View) reviewStatus(msg reviewStatusMsg) tea.Cmd {
	if msg.err != nil {
		return ui.Error(termtext.SafeError(fmt.Errorf("check review state: %w", msg.err)))
	}
	if msg.stage == stageEdit {
		return v.editReview(msg)
	}
	return v.confirmReview(msg)
}

// editReview opens the editor on the saved draft, or the pending review's body.
func (v *View) editReview(msg reviewStatusMsg) tea.Cmd {
	initial := v.reviewDraft
	if initial == "" && msg.status.Pending != nil {
		initial = msg.status.Pending.Body
	}
	owner, repo, number, seq, event := v, v.repo, v.number, v.rseq, msg.event
	return v.edit(initial, func(body string, err error) tea.Msg {
		return reviewEditedMsg{owner: owner, repo: repo, number: number, seq: seq, event: event, body: body, err: err}
	})
}

// reviewEdited keeps the body as the draft, then re-queries the state to
// confirm against.
func (v *View) reviewEdited(msg reviewEditedMsg) tea.Cmd {
	body := strings.TrimRight(msg.body, " \t\r\n")
	if body != "" {
		v.reviewDraft = body
	}
	if msg.err != nil {
		return ui.Error(termtext.SafeError(fmt.Errorf("edit review: %w", msg.err)))
	}
	v.reviewDraft = body
	return v.fetchStatus(stageConfirm, msg.event)
}

// confirmReview applies the checks that need fresh state, then asks.
func (v *View) confirmReview(msg reviewStatusMsg) tea.Cmd {
	st, body := msg.status, v.reviewDraft
	if v.detail != nil && st.HeadSHA != v.detail.HeadSHA {
		owner, repo, number := v, v.repo, v.number
		refresh := func() tea.Msg { return refreshMsg{owner: owner, repo: repo, number: number} }
		return func() tea.Msg {
			return ui.ConfirmMsg{Text: "Head moved since you loaded it; refresh first.", OnYes: refresh, Notice: true}
		}
	}
	inline := 0
	if st.Pending != nil {
		inline = st.Pending.CommentCount
	}
	if msg.event != github.ReviewEventApprove && strings.TrimSpace(body) == "" && inline == 0 {
		return ui.Error(errors.New("empty review, nothing sent (did the editor fail?)"))
	}
	return ui.Confirm(v.reviewQuestion(msg.event, st.Pending), v.submit(msg.event, body, st))
}

// reviewQuestion is the confirmation text.
func (v *View) reviewQuestion(event github.ReviewEvent, pending *github.PendingReview) string {
	return reviewQuestion(event, v.repo, v.number, pending)
}

func reviewQuestion(event github.ReviewEvent, repo github.Repo, number int, pending *github.PendingReview) string {
	where := fmt.Sprintf("#%d in %s", number, termtext.Clean(repo.String()))
	var q string
	switch event {
	case github.ReviewEventApprove:
		q = "Approve " + where
	case github.ReviewEventRequestChanges:
		q = "Request changes on " + where
	case github.ReviewEventComment:
		q = "Comment on " + where
	}
	if pending != nil && pending.CommentCount > 0 {
		noun := "comments"
		if pending.CommentCount == 1 {
			noun = "comment"
		}
		q += fmt.Sprintf(" with %d pending %s", pending.CommentCount, noun)
	}
	return q + "?"
}

// submit is the command that runs when the user confirms. The commit is the
// head the user saw, which confirmReview has just checked is still the head.
// On failure it also returns the error itself, so it shows even if the view
// was left meanwhile.
func (v *View) submit(event github.ReviewEvent, body string, st github.ReviewStatus) tea.Cmd {
	client, repo, number, owner := v.client, v.repo, v.number, v
	in := github.SubmitReviewInput{
		Repo: repo, Number: number, PullRequestID: st.PullRequestID,
		Event: event, Body: body, CommitOID: st.HeadSHA,
	}
	if st.Pending != nil {
		in.PendingReviewID = st.Pending.ID
	}
	return func() tea.Msg {
		err := client.SubmitReview(context.Background(), in)
		res := reviewSubmittedMsg{owner: owner, repo: repo, number: number, event: event, body: body, err: err}
		if err == nil {
			return res
		}
		var shown ui.ErrorMsg
		if errors.Is(err, github.ErrNotFound) && in.PendingReviewID != "" {
			shown = ui.ErrorMsg{Err: errors.New("your pending review was submitted or deleted elsewhere; refreshing, then try again")}
		} else {
			shown = ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("submit review: %w", err))}
		}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

func (v *View) reviewSubmitted(msg reviewSubmittedMsg) tea.Cmd {
	if msg.err != nil {
		// The draft stays for the next attempt. A vanished pending review
		// means the view is out of date.
		if errors.Is(msg.err, github.ErrNotFound) {
			return v.load()
		}
		return nil
	}
	if v.reviewDraft == msg.body {
		v.reviewDraft = ""
	}
	return tea.Batch(ui.Info(fmt.Sprintf(submittedFmt[msg.event], v.number)), v.load())
}

var submittedFmt = map[github.ReviewEvent]string{
	github.ReviewEventApprove:        "Approved #%d",
	github.ReviewEventRequestChanges: "Requested changes on #%d",
	github.ReviewEventComment:        "Commented on #%d",
}
