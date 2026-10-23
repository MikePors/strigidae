package workspace

import (
	"context"
	"errors"
	"fmt"
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

// commentAgreedMsg is what a "yes" produces: the write that was asked about.
// It goes through the view so the write is counted as in flight and refused
// when a review or merge is.
type commentAgreedMsg struct {
	origin origin
	run    tea.Cmd
}

// commentReadMsg is the result of re-reading a comment before the editor
// opens on it: tgt holds the fresh body, or err why it cannot be edited.
type commentReadMsg struct {
	origin origin
	tgt    target
	err    error
}

// staleError is why a comment cannot be changed any more: what is on screen
// is out of date, so the view refreshes.
type staleError string

func (e staleError) Error() string { return string(e) }

const (
	// errCommentGone: the comment does not exist any more.
	errCommentGone = staleError("that comment is gone; refreshed")
	// errNotYours: the viewer has write access, so GitHub lets them change the
	// comment, but somebody else wrote it.
	errNotYours = staleError("that comment is not yours; refreshed")
	// errCannotChange: the viewer wrote the comment but may not change it now
	// (a locked conversation, an archived repository).
	errCannotChange = staleError("you can no longer change that comment; refreshed")
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
		return cm, errCannotChange
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

// answer is what the user has agreed to: nothing yet, or a yes to a question
// asked about a pending (or published) comment.
type answer struct{ given, pending bool }

// yes is the answer to the question asked about a comment in that state.
func yes(pending bool) answer { return answer{given: true, pending: pending} }

// covers reports whether the answer was given for a comment in that state.
func (a answer) covers(pending bool) bool { return a.given && a.pending == pending }

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
	write  func(ctx context.Context, c github.Client) error
	result func(err error) tea.Msg
}

// run re-reads the comment and writes, or asks first. given is what the user
// has already said yes to (the zero answer before any question).
func (w commentWrite) run(given answer) tea.Msg {
	ctx := context.Background()
	fresh, err := freshComment(ctx, w.client, w.id, w.change)
	if err != nil {
		return w.result(err)
	}
	if q := w.ask(fresh.Pending); q != "" && !given.covers(fresh.Pending) {
		pending := fresh.Pending
		again := func() tea.Msg { return w.run(yes(pending)) }
		return commentCheckedMsg{origin: w.origin, question: q, onYes: agreed(w.origin, again)}
	}
	err = w.write(ctx, w.client)
	if errors.Is(err, github.ErrNotFound) {
		err = errCommentGone // deleted between the re-read and the write
	}
	return w.result(err)
}

// agreed wraps the write to run on a yes, so the view sees the yes first.
func agreed(o origin, run tea.Cmd) tea.Cmd {
	return func() tea.Msg { return commentAgreedMsg{origin: o, run: run} }
}

// agreed is agreed for this view's origin.
func (v *View) agreed(run tea.Cmd) tea.Cmd { return agreed(v.origin(), run) }

// commentAgreed starts the write a yes was given for, unless a review is being
// submitted or a merge is running; the write then counts as in flight until its
// result arrives.
func (v *View) commentAgreed(msg commentAgreedMsg) tea.Cmd {
	switch {
	case v.review.submitting:
		return ui.Info("a review is being submitted")
	case v.merge.merging:
		return ui.Info("a merge is in progress")
	}
	v.commentWriting = true
	return msg.run
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
// is being submitted, a comment is being saved, or the pull request or its
// diff is not loaded), or returns nil.
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
// selected range. The comment remembers the commit of the diff its line
// numbers come from.
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
	if r.Start != r.End {
		in.StartLine, in.StartSide = r.Start, r.Side
	}
	return v.compose(target{kind: kindInline, inline: in, head: v.diffHead})
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

// commentAtCursor returns the comment that e/D (or the commands) act on and its
// thread. When there is none, or the view cannot change comments now, the
// command to show why.
func (v *View) commentAtCursor(ch change) (github.ReviewComment, github.ReviewThread, tea.Cmd) {
	if blocked := v.commentBlocked(); blocked != nil {
		return github.ReviewComment{}, github.ReviewThread{}, blocked
	}
	t, _, hint := v.focusedThread("") // no thread: pickComment finds nothing
	if hint != nil {
		return github.ReviewComment{}, github.ReviewThread{}, hint
	}
	c, found := pickComment(t, ch)
	if !found {
		return github.ReviewComment{}, github.ReviewThread{}, ui.Info("no comment of yours here")
	}
	return c, t, nil
}

// editComment re-reads the viewer's comment at the cursor, then opens the
// editor on its current body, so edits made elsewhere are not overwritten.
func (v *View) editComment() tea.Cmd {
	c, t, cmd := v.commentAtCursor(changeEdit)
	if cmd != nil {
		return cmd
	}
	o, client := v.origin(), v.client
	tgt := target{kind: kindEdit, commentID: c.ID, threadID: t.ID, label: threadLocation(t)}
	return func() tea.Msg {
		fresh, err := freshComment(context.Background(), client, tgt.commentID, changeEdit)
		tgt.initial = fresh.Body
		return commentReadMsg{origin: o, tgt: tgt, err: err}
	}
}

// commentRead opens the editor on a freshly read comment, or refuses.
func (v *View) commentRead(msg commentReadMsg) tea.Cmd {
	if msg.err != nil {
		return v.refuse(msg.err, "read comment")
	}
	if blocked := v.commentBlocked(); blocked != nil {
		return blocked
	}
	return v.compose(msg.tgt)
}

// refuse shows why a comment cannot be changed, and refreshes when what is on
// screen turned out to be stale.
func (v *View) refuse(err error, what string) tea.Cmd {
	e := ui.Error(commentFailure(err, what))
	if isStale(err) {
		return tea.Batch(e, v.reloadPendingComments())
	}
	return e
}

// deleteComment re-reads the comment at the cursor, then asks before deleting
// it. The question says whether it is pending or published.
func (v *View) deleteComment() tea.Cmd {
	c, t, cmd := v.commentAtCursor(changeDelete)
	if cmd != nil {
		return cmd
	}
	o, id, label := v.origin(), c.ID, termtext.Clean(threadLocation(t))
	w := commentWrite{
		origin: o, client: v.client, id: id, change: changeDelete,
		ask: func(pending bool) string {
			if pending {
				return "Delete pending comment on " + label + "?"
			}
			return fmt.Sprintf("Delete your published comment on #%d %s?", o.number, label)
		},
		write: func(ctx context.Context, cl github.Client) error { return cl.DeleteReviewComment(ctx, id) },
		result: func(err error) tea.Msg {
			res := commentDeletedMsg{origin: o, err: err}
			if err == nil {
				return res
			}
			return withShownError(res, commentFailure(err, "delete comment"))
		},
	}
	v.commentWriting = true
	return func() tea.Msg { return w.run(answer{}) }
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
	v.commentWriting = false
	if isStale(msg.err) {
		return v.reloadPendingComments() // the error is already shown
	}
	if msg.err != nil {
		return nil
	}
	return tea.Batch(ui.Info("Comment deleted"), v.reloadPendingComments())
}

// sendEdit updates a comment of the viewer's. Updating a pending comment runs
// without a prompt; updating a published one asks first. A failed or refused
// edit never throws the text away (see editFailure and staleEdit).
func (v *View) sendEdit(tgt target, body string) tea.Cmd {
	// What the editor returned is trimmed; so is what was offered to it.
	if body == strings.TrimRight(tgt.initial, " \t\r\n") {
		delete(v.drafts, tgt.draftKey())
		return ui.Info("comment unchanged")
	}
	o, client, label := v.origin(), v.client, termtext.Clean(tgt.label)
	w := commentWrite{
		origin: o, client: client, id: tgt.commentID, change: changeEdit,
		ask: func(pending bool) string {
			if pending {
				return ""
			}
			return fmt.Sprintf("Update your published comment on #%d %s?", o.number, label)
		},
		write: func(ctx context.Context, cl github.Client) error {
			_, err := cl.UpdateReviewComment(ctx, tgt.commentID, body)
			return err
		},
		result: func(err error) tea.Msg {
			res := postedMsg{origin: o, tgt: tgt, body: body, err: err}
			if err == nil {
				return res
			}
			return withShownError(res, editFailure(err))
		},
	}
	v.commentWriting = true
	return func() tea.Msg { return w.run(answer{}) }
}

// editFailure is the error shown for a failed edit, saying where the text is
// kept: a comment that is gone gets its text as a reply draft on its thread.
func editFailure(err error) error {
	var s staleError
	switch {
	case errors.Is(err, errCommentGone):
		return staleError(string(errCommentGone) + "; your text is kept as a reply draft")
	case errors.As(err, &s):
		return staleError(string(s) + "; your text is kept as a draft")
	}
	return commentFailure(fmt.Errorf("%w; the comment is kept as a draft", err), "update comment")
}

// staleEdit handles an edit that was refused because its comment is stale: it
// refreshes, and when the comment is gone it moves the text to the reply draft
// of its thread, so `r` there starts from it. Otherwise the text stays under
// the comment's own draft key, where the next e finds it. (If the thread is
// gone after the refresh, the text is lost with it.)
func (v *View) staleEdit(msg postedMsg) tea.Cmd {
	if errors.Is(msg.err, errCommentGone) && msg.tgt.threadID != "" {
		reply := target{kind: kindReply, threadID: msg.tgt.threadID}.draftKey()
		text := msg.body
		if old := v.drafts[reply]; old != "" && old != text {
			text = old + "\n\n" + text
		}
		v.drafts[reply] = text
		delete(v.drafts, msg.tgt.draftKey())
	}
	return v.reloadPendingComments()
}

// sendInline adds a new inline comment to the pending review (creating the
// review if there is none), anchored to the commit the lines were picked on.
func (v *View) sendInline(tgt target, body string) tea.Cmd {
	if v.detail == nil || v.diffPending {
		return ui.Error(errors.New("the diff is refreshing; the comment is kept as a draft, try again"))
	}
	o, client := v.origin(), v.client
	in := *tgt.inline
	in.Repo, in.Number, in.Body = o.repo, o.number, body
	prID := v.detail.ID
	v.commentWriting = true
	return func() tea.Msg {
		err := addPendingThread(context.Background(), client, in, prID, tgt.head)
		res := postedMsg{origin: o, tgt: tgt, body: body, err: err}
		if err == nil {
			return res
		}
		return withShownError(res, commentFailure(fmt.Errorf("%w; the comment is kept as a draft", err), "add pending comment"))
	}
}

// addPendingThread adds in to the viewer's pending review, creating it on
// head first when there is none. The comment's line numbers come from the diff
// of commit head, so it is refused when the pull request has moved on or the
// existing pending review is anchored to another (or an unknown) commit.
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
	case pending.CommitOID == "":
		return errors.New("your pending review is on an unknown commit; submit or discard it on GitHub first")
	case pending.CommitOID != head:
		return errors.New("your pending review is on an older commit; submit it first, or delete it on GitHub")
	}
	in.PendingReviewID = pending.ID
	_, err = c.AddReviewThread(ctx, in)
	return err
}
