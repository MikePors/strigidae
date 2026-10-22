package workspace

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// Inline comments are written on diff lines and collect in the viewer's
// pending review. Adding or editing a pending comment asks nothing (private
// drafts). Deleting any comment, and editing a published one, asks y/N.

// commentDeletedMsg is the result of deleting a comment. Like a post it is
// identified by owner, repo and number only.
type commentDeletedMsg struct {
	owner  *View
	repo   github.Repo
	number int
	err    error
}

// errCommentGone is returned when a comment to change does not exist any more
// or the viewer may no longer change it.
var errCommentGone = errors.New("that comment is gone or no longer yours; refreshed")

// commentCheckedMsg is the result of re-reading a comment before a prompt:
// either the confirmation to show, or why the action is refused.
type commentCheckedMsg struct {
	owner   *View
	repo    github.Repo
	number  int
	confirm ui.ConfirmMsg
	err     error
}

// freshComment re-reads comment id and returns it, or errCommentGone when it
// is missing or the viewer may not update (or, with deleting, delete) it. The
// fresh state, not the one on screen, decides what to ask.
func freshComment(ctx context.Context, c github.Client, id string, deleting bool) (github.ReviewComment, error) {
	cm, err := c.GetReviewComment(ctx, id)
	switch {
	case errors.Is(err, github.ErrNotFound):
		return cm, errCommentGone
	case err != nil:
		return cm, fmt.Errorf("check comment: %w", err)
	case deleting && !cm.ViewerCanDelete, !deleting && !cm.ViewerCanUpdate:
		return cm, errCommentGone
	}
	return cm, nil
}

// reloadPendingComments refreshes what shows pending comments: the threads in
// the diff, and the pull request (whose reviews list the pending review).
func (v *View) reloadPendingComments() tea.Cmd {
	return tea.Batch(v.reloadPR(), v.reloadThreads())
}

// inlineComment starts a comment on the diff line under the cursor, or on the
// selected range.
func (v *View) inlineComment() tea.Cmd {
	if blocked := v.review.blocked(v.reviewEnv()); blocked != nil {
		return blocked
	}
	r, ok := v.filePane.CommentRange()
	if !ok {
		return ui.Info("move onto a diff line to comment on it")
	}
	if v.filePane.VisualActive() {
		v.filePane.VisualEnd()
	}
	in := &github.AddThreadInput{Path: r.Path, Line: r.End, Side: r.Side}
	label := r.Path + ":" + strconv.Itoa(r.End)
	if r.Start != r.End {
		in.StartLine, in.StartSide = r.Start, r.Side
		label = r.Path + ":" + strconv.Itoa(r.Start) + "-" + strconv.Itoa(r.End)
	}
	return v.compose(target{inline: in, label: label})
}

// startVisual enters visual-line mode on the diff line under the cursor.
func (v *View) startVisual() tea.Cmd {
	if v.sel() == 0 {
		return nil
	}
	if blocked := v.review.blocked(v.reviewEnv()); blocked != nil {
		return blocked
	}
	if !v.filePane.VisualStart() {
		return ui.Info("move onto a diff line to select lines")
	}
	v.focus = paneRight
	return ui.Info("-- VISUAL LINE --  j/k extend, c comment, esc cancel")
}

// visualKey handles a raw key while a line selection is in progress. The
// workspace captures keys then, and esc always leaves.
func (v *View) visualKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, v.bind.VisualExit):
		v.filePane.VisualEnd()
	case key.Matches(msg, v.bind.VisualDown):
		return refusal(v.filePane.VisualMove(1))
	case key.Matches(msg, v.bind.VisualUp):
		return refusal(v.filePane.VisualMove(-1))
	case key.Matches(msg, v.bind.Comment):
		return v.inlineComment()
	}
	return nil
}

func refusal(reason string) tea.Cmd {
	if reason == "" {
		return nil
	}
	return ui.Info(reason)
}

// commentAtCursor returns the comment the viewer may change that e/D (or the
// commands) act on: the last one of the focused thread, preferring pending
// comments over published ones. When there is none, the command to show why.
func (v *View) commentAtCursor(deleting bool) (github.ReviewComment, github.ReviewThread, tea.Cmd) {
	var none github.ReviewComment
	if blocked := v.review.blocked(v.reviewEnv()); blocked != nil {
		return none, github.ReviewThread{}, blocked
	}
	var ts []github.ReviewThread
	if v.sel() > 0 {
		ts = v.filePane.FocusedThreads()
	}
	if len(ts) > 1 {
		return none, github.ReviewThread{}, ui.Info("several threads on this line: move onto one")
	}
	if len(ts) == 0 {
		return none, github.ReviewThread{}, ui.Info("no comment of yours here")
	}
	t := ts[0]
	var found, pending *github.ReviewComment
	for i := range t.Comments {
		c := &t.Comments[i]
		if (deleting && !c.ViewerCanDelete) || (!deleting && !c.ViewerCanUpdate) {
			continue
		}
		found = c
		if c.Pending {
			pending = c
		}
	}
	if pending != nil {
		found = pending
	}
	if found == nil {
		return none, t, ui.Info("no comment of yours here")
	}
	return *found, t, nil
}

// editComment opens the editor on the viewer's comment at the cursor.
func (v *View) editComment() tea.Cmd {
	c, t, cmd := v.commentAtCursor(false)
	if c.ID == "" {
		return cmd
	}
	return v.compose(target{commentID: c.ID, initial: c.Body, label: threadLocation(t)})
}

// deleteComment re-reads the comment at the cursor, then asks before deleting
// it. The question says whether it is pending or published.
func (v *View) deleteComment() tea.Cmd {
	c, t, cmd := v.commentAtCursor(true)
	if c.ID == "" {
		return cmd
	}
	client, repo, number, owner, id, label := v.client, v.repo, v.number, v, c.ID, termtext.Clean(threadLocation(t))
	return func() tea.Msg {
		res := commentCheckedMsg{owner: owner, repo: repo, number: number}
		fresh, err := freshComment(context.Background(), client, id, true)
		if err != nil {
			res.err = err
			return res
		}
		q := "Delete your published comment on " + label + "?"
		if fresh.Pending {
			q = "Delete pending comment on " + label + "?"
		}
		res.confirm = ui.ConfirmMsg{Text: q, OnYes: sendDelete(client, owner, repo, number, id)}
		return res
	}
}

// sendDelete runs when the user confirms: it re-reads the comment once more
// (the answer may have taken a while), then deletes it.
func sendDelete(client github.Client, owner *View, repo github.Repo, number int, id string) tea.Cmd {
	return func() tea.Msg {
		_, err := freshComment(context.Background(), client, id, true)
		if err == nil {
			err = client.DeleteReviewComment(context.Background(), id)
		}
		res := commentDeletedMsg{owner: owner, repo: repo, number: number, err: err}
		if err == nil {
			return res
		}
		shown := ui.ErrorMsg{Err: commentFailure(err, "delete comment")}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

// commentChecked shows the prompt a check produced, or reports the refusal.
func (v *View) commentChecked(msg commentCheckedMsg) tea.Cmd {
	if msg.err != nil {
		return v.refuseStale(msg.err, "check comment")
	}
	return ui.Confirm(msg.confirm.Text, msg.confirm.OnYes)
}

// refuseStale reports a failed check, refreshing when the comment turned out
// to be gone.
func (v *View) refuseStale(err error, what string) tea.Cmd {
	e := ui.Error(commentFailure(err, what))
	if errors.Is(err, errCommentGone) {
		return tea.Batch(e, v.reloadPendingComments())
	}
	return e
}

func commentFailure(err error, what string) error {
	if errors.Is(err, errCommentGone) {
		return errCommentGone
	}
	return termtext.SafeError(fmt.Errorf("%s: %w", what, err))
}

func (v *View) commentDeleted(msg commentDeletedMsg) tea.Cmd {
	if errors.Is(msg.err, errCommentGone) {
		return v.reloadPendingComments() // the error is already shown
	}
	if msg.err != nil {
		return nil
	}
	return tea.Batch(ui.Info("Comment deleted"), v.reloadPendingComments())
}

// commentSend adds a new inline comment to the pending review (creating the
// review if there is none), or updates one. Adding, and updating a pending
// comment, run without a prompt; updating a published one asks first.
func (v *View) commentSend(tgt target, body string) tea.Cmd {
	client, repo, number, owner := v.client, v.repo, v.number, v
	result := func(err error, what string) tea.Msg {
		res := postedMsg{owner: owner, repo: repo, number: number, tgt: tgt, body: body, err: err}
		if err == nil {
			return res
		}
		shown := ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("%s: %w; the comment is kept as a draft", what, err))}
		if errors.Is(err, errCommentGone) {
			shown = ui.ErrorMsg{Err: errCommentGone}
		}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
	if tgt.commentID != "" {
		if body == tgt.initial {
			delete(v.drafts, tgt.draftKey())
			return ui.Info("comment unchanged")
		}
		update := func() error {
			_, err := client.UpdateReviewComment(context.Background(), tgt.commentID, body)
			return err
		}
		return func() tea.Msg {
			// Fresh state decides: the comment may have been submitted, changed
			// hands or been deleted while the editor was open.
			fresh, err := freshComment(context.Background(), client, tgt.commentID, false)
			if err != nil {
				return result(err, "update comment")
			}
			if fresh.Pending {
				return result(update(), "update pending comment")
			}
			send := func() tea.Msg {
				_, err := freshComment(context.Background(), client, tgt.commentID, false)
				if err == nil {
					err = update()
				}
				return result(err, "update comment")
			}
			return ui.ConfirmMsg{Text: "Update your published comment on " + termtext.Clean(tgt.label) + "?", OnYes: send}
		}
	}
	if v.detail == nil || v.diffPending {
		return ui.Error(errors.New("the diff is refreshing; the comment is kept as a draft, try again"))
	}
	in := *tgt.inline
	in.Repo, in.Number, in.Body = repo, number, body
	prID, head := v.detail.ID, v.diffHead
	return func() tea.Msg {
		return result(addPendingThread(context.Background(), client, in, prID, head), "add pending comment")
	}
}

// addPendingThread adds in to the viewer's pending review, creating it on
// head first when there is none. The comment's line numbers come from the diff
// of commit head, so it is refused when the pull request has moved on or the
// existing pending review is anchored to another commit.
func addPendingThread(ctx context.Context, c github.Client, in github.AddThreadInput, prID, head string) error {
	st, err := c.GetReviewStatus(ctx, in.Repo, in.Number)
	if err != nil {
		return fmt.Errorf("check review state: %w", err)
	}
	if st.HeadSHA != head {
		return errors.New("head moved since you loaded the diff; refresh first")
	}
	pending := st.Pending
	switch {
	case pending == nil:
		p, err := c.StartReview(ctx, github.StartReviewInput{Repo: in.Repo, Number: in.Number, PullRequestID: prID, CommitOID: head})
		if err != nil {
			return fmt.Errorf("start review: %w", err)
		}
		pending = &p
	case pending.CommitOID != "" && pending.CommitOID != head:
		return errors.New("your pending review is on an older commit; submit it first, or delete it on GitHub")
	}
	in.PendingReviewID = pending.ID
	_, err = c.AddReviewThread(ctx, in)
	return err
}
