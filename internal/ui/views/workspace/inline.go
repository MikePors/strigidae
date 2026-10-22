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
// pending review. They are private drafts, so adding or editing one asks for
// no confirmation; deleting one does, because its text is lost.

// pendingDeletedMsg is the result of deleting a pending comment. Like a post
// it is identified by owner, repo and number only.
type pendingDeletedMsg struct {
	owner  *View
	repo   github.Repo
	number int
	err    error
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

// pendingAtCursor returns the viewer's pending comment under the cursor (the
// last one of the thread), and the thread it is in.
func (v *View) pendingAtCursor() (github.ReviewComment, github.ReviewThread, tea.Cmd) {
	var none github.ReviewComment
	if v.sel() == 0 {
		return none, github.ReviewThread{}, nil
	}
	ts := v.filePane.FocusedThreads()
	switch len(ts) {
	case 0:
		return none, github.ReviewThread{}, ui.Info("no comment thread here")
	case 1:
	default:
		return none, github.ReviewThread{}, ui.Info("several threads on this line: move onto one")
	}
	t := ts[0]
	for i := len(t.Comments) - 1; i >= 0; i-- {
		if t.Comments[i].Pending {
			return t.Comments[i], t, nil
		}
	}
	return none, t, ui.Info("no pending comment in this thread: only your unsubmitted comments can be changed")
}

// editPending opens the editor on the pending comment under the cursor.
func (v *View) editPending() tea.Cmd {
	c, t, cmd := v.pendingAtCursor()
	if c.ID == "" {
		return cmd
	}
	return v.compose(target{commentID: c.ID, initial: c.Body, label: threadLocation(t)})
}

// deletePending asks, then deletes the pending comment under the cursor.
func (v *View) deletePending() tea.Cmd {
	c, t, cmd := v.pendingAtCursor()
	if c.ID == "" {
		return cmd
	}
	q := "Delete pending comment on " + termtext.Clean(threadLocation(t)) + "?"
	client, repo, number, owner, id := v.client, v.repo, v.number, v, c.ID
	del := func() tea.Msg {
		err := client.DeleteReviewComment(context.Background(), id)
		res := pendingDeletedMsg{owner: owner, repo: repo, number: number, err: err}
		if err == nil {
			return res
		}
		shown := ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("delete pending comment: %w", err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
	return ui.Confirm(q, del)
}

func (v *View) pendingDeleted(msg pendingDeletedMsg) tea.Cmd {
	if msg.err != nil && !errors.Is(msg.err, github.ErrNotFound) {
		return nil
	}
	// A comment that is already gone is shown as gone.
	info := ui.Info("Pending comment deleted")
	if msg.err != nil {
		info = nil
	}
	return tea.Batch(info, v.reloadPendingComments())
}

// pendingCommentSend adds a new inline comment to the pending review (creating
// the review if there is none), or updates one. It runs without confirmation.
func (v *View) pendingCommentSend(tgt target, body string) tea.Cmd {
	client, repo, number, owner := v.client, v.repo, v.number, v
	result := func(err error, what string) tea.Msg {
		res := postedMsg{owner: owner, repo: repo, number: number, tgt: tgt, body: body, err: err}
		if err == nil {
			return res
		}
		shown := ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("%s: %w; the comment is kept as a draft", what, err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
	if tgt.commentID != "" {
		if body == tgt.initial {
			delete(v.drafts, tgt.draftKey())
			return ui.Info("comment unchanged")
		}
		return func() tea.Msg {
			_, err := client.UpdateReviewComment(context.Background(), tgt.commentID, body)
			return result(err, "update pending comment")
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
