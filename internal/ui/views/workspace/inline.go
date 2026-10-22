package workspace

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// Inline comments are written on diff lines and collect in the viewer's
// pending review. Adding or editing a pending comment asks nothing (private
// drafts). Deleting any comment, and editing a published one, asks y/N.

// commentDeletedMsg is the result of deleting a comment (or of a refusal
// before the question). Like a post it is identified by its origin only.
type commentDeletedMsg struct {
	origin origin
	err    error
}

// commentCheckedMsg carries a question for the user, asked after a comment
// was re-read: onYes runs on yes.
type commentCheckedMsg struct {
	origin   origin
	question string
	onYes    tea.Cmd
}

// staleError is why a comment cannot be changed any more: what is on screen
// is out of date, so the view refreshes.
type staleError string

func (e staleError) Error() string { return string(e) }

const (
	// errCommentGone: the comment does not exist any more, or the viewer may
	// no longer change it.
	errCommentGone = staleError("that comment is gone or no longer yours; refreshed")
	// errNotYours: the viewer has write access, so GitHub lets them change the
	// comment, but somebody else wrote it.
	errNotYours = staleError("that comment is not yours; refreshed")
)

// isStale reports whether err says the comment on screen is out of date.
func isStale(err error) bool {
	var s staleError
	return errors.As(err, &s)
}

// change is what is done to a comment.
type change int

const (
	changeEdit change = iota
	changeDelete
)

// allowed reports whether the viewer may make the change: they wrote the
// comment and GitHub allows it. Write access makes the can-flags true on other
// people's comments too, so authorship is checked as well.
func (ch change) allowed(c github.ReviewComment) bool {
	if !c.ViewerDidAuthor {
		return false
	}
	if ch == changeDelete {
		return c.ViewerCanDelete
	}
	return c.ViewerCanUpdate
}

// freshComment re-reads comment id and returns it, or a staleError when it is
// missing, not the viewer's, or not changeable by them. The fresh state, not
// the one on screen, decides what to ask.
func freshComment(ctx context.Context, c github.Client, id string, ch change) (github.ReviewComment, error) {
	cm, err := c.GetReviewComment(ctx, id)
	switch {
	case errors.Is(err, github.ErrNotFound):
		return cm, errCommentGone
	case err != nil:
		return cm, fmt.Errorf("check comment: %w", err)
	case !cm.ViewerDidAuthor:
		return cm, errNotYours
	case !ch.allowed(cm):
		return cm, errCommentGone
	}
	return cm, nil
}

// pickComment is the comment of t that e/D act on: the last one the viewer may
// change, but a pending comment beats a published one.
func pickComment(t github.ReviewThread, ch change) (github.ReviewComment, bool) {
	var picked github.ReviewComment
	var found bool
	for _, c := range t.Comments {
		if !ch.allowed(c) {
			continue
		}
		if !found || c.Pending || !picked.Pending {
			picked, found = c, true
		}
	}
	return picked, found
}

// commentWrite is one guarded write to a comment of the viewer's. It re-reads
// the comment first and asks before writing when ask has a question for the
// comment's state right now; the answer is good only for the state that was
// asked about. A comment that changed state while the question was open is
// asked about again, so nothing is written on a stale yes.
type commentWrite struct {
	origin origin
	client github.Client
	id     string
	change change
	// ask returns the question for a comment that is pending or published, or
	// "" to write without asking.
	ask    func(pending bool) string
	write  func(ctx context.Context) error
	result func(err error) tea.Msg
}

// run re-reads the comment and writes, or asks first. agreed is the state the
// user said yes for (nil before any question).
func (w commentWrite) run(agreed *bool) tea.Msg {
	ctx := context.Background()
	fresh, err := freshComment(ctx, w.client, w.id, w.change)
	if err != nil {
		return w.result(err)
	}
	if q := w.ask(fresh.Pending); q != "" && (agreed == nil || *agreed != fresh.Pending) {
		pending := fresh.Pending
		return commentCheckedMsg{origin: w.origin, question: q, onYes: func() tea.Msg { return w.run(&pending) }}
	}
	return w.result(w.write(ctx))
}

// withShownError returns res together with an error to show, so the error
// shows even if the view was left meanwhile.
func withShownError(res tea.Msg, shown error) tea.Msg {
	return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return ui.ErrorMsg{Err: shown} }}
}

// reloadPendingComments refreshes what shows pending comments: the threads in
// the diff, and the pull request (whose reviews list the pending review).
func (v *View) reloadPendingComments() tea.Cmd {
	return tea.Batch(v.reloadPR(), v.reloadThreads())
}

// commentBlocked says why comments cannot be written or changed now (a review
// is being submitted, or the pull request or its diff is not loaded), or
// returns nil.
func (v *View) commentBlocked() tea.Cmd { return v.review.blocked(v.reviewEnv()) }

// focusedThread returns the one thread under the diff cursor. ok is false when
// there is none (or the diff is not showing); hint is the message to show when
// several threads share the line. what finishes that message.
func (v *View) focusedThread(what string) (t github.ReviewThread, ok bool, hint tea.Cmd) {
	if v.sel() == 0 {
		return t, false, nil
	}
	switch ts := v.filePane.FocusedThreads(); len(ts) {
	case 0:
		return t, false, nil
	case 1:
		return ts[0], true, nil
	}
	return t, false, ui.Info("several threads on this line: move onto one" + what)
}

// inlineComment starts a comment on the diff line under the cursor, or on the
// selected range.
func (v *View) inlineComment() tea.Cmd {
	if blocked := v.commentBlocked(); blocked != nil {
		return blocked
	}
	r, ok := v.filePane.CommentRange()
	if !ok {
		return ui.Info("move onto a diff line to comment on it")
	}
	v.filePane.VisualEnd()
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
	if blocked := v.commentBlocked(); blocked != nil {
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

func refusal(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return ui.Info(err.Error())
}

// commentAtCursor returns the comment that e/D (or the commands) act on, and
// the "path:line" of its thread. When there is none, or the view cannot change
// comments now, the command to show why.
func (v *View) commentAtCursor(ch change) (github.ReviewComment, string, tea.Cmd) {
	if blocked := v.commentBlocked(); blocked != nil {
		return github.ReviewComment{}, "", blocked
	}
	t, ok, hint := v.focusedThread("")
	if hint != nil {
		return github.ReviewComment{}, "", hint
	}
	if !ok {
		return github.ReviewComment{}, "", ui.Info("no comment of yours here")
	}
	c, found := pickComment(t, ch)
	if !found {
		return github.ReviewComment{}, "", ui.Info("no comment of yours here")
	}
	return c, threadLocation(t), nil
}

// editComment opens the editor on the viewer's comment at the cursor.
func (v *View) editComment() tea.Cmd {
	c, label, cmd := v.commentAtCursor(changeEdit)
	if cmd != nil {
		return cmd
	}
	return v.compose(target{commentID: c.ID, initial: c.Body, label: label})
}

// deleteComment re-reads the comment at the cursor, then asks before deleting
// it. The question says whether it is pending or published.
func (v *View) deleteComment() tea.Cmd {
	c, label, cmd := v.commentAtCursor(changeDelete)
	if cmd != nil {
		return cmd
	}
	o, label := v.origin(), termtext.Clean(label)
	w := commentWrite{
		origin: o, client: v.client, id: c.ID, change: changeDelete,
		ask: func(pending bool) string {
			if pending {
				return "Delete pending comment on " + label + "?"
			}
			return fmt.Sprintf("Delete your published comment on #%d %s?", o.number, label)
		},
		write: func(ctx context.Context) error { return v.client.DeleteReviewComment(ctx, c.ID) },
		result: func(err error) tea.Msg {
			res := commentDeletedMsg{origin: o, err: err}
			if err == nil {
				return res
			}
			return withShownError(res, commentFailure(err, "delete comment"))
		},
	}
	return func() tea.Msg { return w.run(nil) }
}

// commentChecked shows the question a check produced.
func (v *View) commentChecked(msg commentCheckedMsg) tea.Cmd {
	return ui.Confirm(msg.question, msg.onYes)
}

// commentFailure is the error to show for a failed comment call: the stale
// reason as it is, any other wrapped with what was being done.
func commentFailure(err error, what string) error {
	var s staleError
	if errors.As(err, &s) {
		return s
	}
	return termtext.SafeError(fmt.Errorf("%s: %w", what, err))
}

func (v *View) commentDeleted(msg commentDeletedMsg) tea.Cmd {
	if isStale(msg.err) {
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
	o, client := v.origin(), v.client
	result := func(err error, what string) tea.Msg {
		res := postedMsg{origin: o, tgt: tgt, body: body, err: err}
		if err == nil {
			return res
		}
		return withShownError(res, commentFailure(fmt.Errorf("%w; the comment is kept as a draft", err), what))
	}
	if tgt.commentID != "" {
		// What the editor returned is trimmed; so is what was offered to it.
		if body == strings.TrimRight(tgt.initial, " \t\r\n") {
			delete(v.drafts, tgt.draftKey())
			return ui.Info("comment unchanged")
		}
		label := termtext.Clean(tgt.label)
		w := commentWrite{
			origin: o, client: client, id: tgt.commentID, change: changeEdit,
			ask: func(pending bool) string {
				if pending {
					return ""
				}
				return fmt.Sprintf("Update your published comment on #%d %s?", o.number, label)
			},
			write: func(ctx context.Context) error {
				_, err := client.UpdateReviewComment(ctx, tgt.commentID, body)
				return err
			},
			result: func(err error) tea.Msg { return result(err, "update comment") },
		}
		return func() tea.Msg { return w.run(nil) }
	}
	if v.detail == nil || v.diffPending {
		return ui.Error(errors.New("the diff is refreshing; the comment is kept as a draft, try again"))
	}
	in := *tgt.inline
	in.Repo, in.Number, in.Body = o.repo, o.number, body
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
