package workspace

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// Inline comments are written on diff lines and collect in the viewer's
// pending review. Adding or editing a pending comment asks nothing (private
// drafts). Deleting any comment, and editing a published one, asks y/N.

// commentDeletedMessage is the result of deleting a comment (or of a refusal
// before the question). Like a post it is identified by its origin only.
type commentDeletedMessage struct {
	origin origin
	err    error
}

// commentCheckedMessage carries a question for the user, asked after a comment
// was re-read: onYes runs on yes.
type commentCheckedMessage struct {
	origin   origin
	question string
	onYes    tea.Cmd
}

// commentAgreedMessage is what a "yes" produces: the write that was asked about.
// It goes through the view so the write is counted as in flight and refused
// when a review or merge is.
type commentAgreedMessage struct {
	origin origin
	run    tea.Cmd
}

// commentReadMessage is the result of re-reading a comment before the editor
// opens on it: tgt holds the fresh body, or err why it cannot be edited.
type commentReadMessage struct {
	origin   origin
	sequence int // the compose sequence it was asked under; a later compose supersedes it
	target   target
	err      error
}

// staleError is why a comment cannot be changed any more: what is on screen
// is out of date, so the view refreshes.
type staleError string

func (staleError staleError) Error() string { return string(staleError) }

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
	var staleFailure staleError
	return errors.As(err, &staleFailure)
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
func (change change) allowed(comment github.ReviewComment) bool {
	if !comment.ViewerDidAuthor {
		return false
	}
	if change == changeDelete {
		return comment.ViewerCanDelete
	}
	return comment.ViewerCanUpdate
}

// freshComment re-reads comment identifier and returns it, or a staleError when it is
// missing, not the viewer's, or not changeable by them. The fresh state, not
// the one on screen, decides what to ask.
func freshComment(ctx context.Context, client github.Client, identifier string, change change) (github.ReviewComment, error) {
	comment, err := client.GetReviewComment(ctx, identifier)
	switch {
	case errors.Is(err, github.ErrNotFound):
		return comment, errCommentGone
	case err != nil:
		return comment, fmt.Errorf("check comment: %w", err)
	case !comment.ViewerDidAuthor:
		return comment, errNotYours
	case !change.allowed(comment):
		return comment, errCannotChange
	}
	return comment, nil
}

// pickComment is the comment of t that e/D act on: the last one the viewer may
// change, but a pending comment beats a published one.
func pickComment(thread github.ReviewThread, change change) (github.ReviewComment, bool) {
	var picked github.ReviewComment
	var found bool
	for _, comment := range thread.Comments {
		if !change.allowed(comment) {
			continue
		}
		if !found || comment.Pending || !picked.Pending {
			picked, found = comment, true
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
func (answer answer) covers(pending bool) bool { return answer.given && answer.pending == pending }

// commentWrite is one guarded write to a comment of the viewer's. It re-reads
// the comment first and asks before writing when ask has a question for the
// comment's state right now; the answer is good only for the state that was
// asked about. A comment that changed state while the question was open is
// asked about again, so nothing is written on a stale yes.
type commentWrite struct {
	origin     origin
	client     github.Client
	timeouts   userinterface.Timeouts
	identifier string
	change     change
	// ask returns the question for a comment that is pending or published, or
	// "" to write without asking.
	ask    func(pending bool) string
	write  func(ctx context.Context, client github.Client) error
	result func(err error) tea.Msg
}

// run re-reads the comment and writes, or asks first. given is what the user
// has already said yes to (the zero answer before any question).
func (write commentWrite) run(given answer) tea.Msg {
	readContext, cancelRead := write.timeouts.RequestContext()
	defer cancelRead()
	fresh, err := freshComment(readContext, write.client, write.identifier, write.change)
	if err != nil {
		return write.result(err)
	}
	if query := write.ask(fresh.Pending); query != "" && !given.covers(fresh.Pending) {
		pending := fresh.Pending
		again := func() tea.Msg { return write.run(yes(pending)) }
		return commentCheckedMessage{origin: write.origin, question: query, onYes: agreed(write.origin, again)}
	}
	writeContext, cancelWrite := write.timeouts.RequestContext()
	defer cancelWrite()
	err = userinterface.WriteError(write.write(writeContext, write.client))
	if errors.Is(err, github.ErrNotFound) {
		err = errCommentGone // deleted between the re-read and the write
	}
	return write.result(err)
}

// agreed wraps the write to run on a yes, so the view sees the yes first.
func agreed(origin origin, run tea.Cmd) tea.Cmd {
	return func() tea.Msg { return commentAgreedMessage{origin: origin, run: run} }
}

// agreed is agreed for this view's origin.
func (view *View) agreed(run tea.Cmd) tea.Cmd { return agreed(view.origin(), run) }

// commentAgreed starts the write a yes was given for, unless a review is being
// submitted or a merge is running; the write then counts as in flight until its
// result arrives.
func (view *View) commentAgreed(message commentAgreedMessage) tea.Cmd {
	switch {
	case view.review.submitting:
		return userinterface.Info("a review is being submitted")
	case view.merge.merging:
		return userinterface.Info("a merge is in progress")
	case view.writes > 0:
		return userinterface.Info(commentSavingText)
	}
	view.writeStarted()
	return message.run
}

// withShownError returns result together with an error to show, so the error
// shows even if the view was left meanwhile.
func withShownError(result tea.Msg, shown error) tea.Msg {
	return tea.BatchMsg{func() tea.Msg { return result }, func() tea.Msg { return userinterface.ErrorMessage{Err: shown} }}
}

// writeStarted and writeDone count the comment writes in flight; the count
// never goes below zero.
func (view *View) writeStarted() { view.writes++ }

func (view *View) writeDone() { view.writes = max(0, view.writes-1) }

// writeBusy refuses a new comment write while one is in flight.
func (view *View) writeBusy() tea.Cmd {
	if view.writes > 0 {
		return userinterface.Info(commentSavingText)
	}
	return nil
}

// reloadPendingComments refreshes what shows pending comments: the threads in
// the diff, and the pull request (whose reviews list the pending review).
func (view *View) reloadPendingComments() tea.Cmd {
	return tea.Batch(view.reloadPullRequest(), view.reloadThreads())
}

// commentBlocked says why comments cannot be written or changed now (a review
// is being submitted, a comment is being saved, or the pull request or its
// diff is not loaded), or returns nil.
func (view *View) commentBlocked() tea.Cmd { return view.review.blocked(view.reviewEnvironment()) }

// focusedThread returns the one thread under the diff cursor. ok is false when
// there is none (or the diff is not showing); hint is the message to show when
// several threads share the line. what finishes that message.
func (view *View) focusedThread(what string) (thread github.ReviewThread, ok bool, hint tea.Cmd) {
	if view.selection() == 0 {
		return thread, false, nil
	}
	switch threads := view.filePane.FocusedThreads(); len(threads) {
	case 0:
		return thread, false, nil
	case 1:
		return threads[0], true, nil
	}
	return thread, false, userinterface.Info("several threads on this line: move onto one" + what)
}

// inlineComment starts a comment on the diff line under the cursor, or on the
// selected range. The comment remembers the commit of the diff its line
// numbers come from.
func (view *View) inlineComment() tea.Cmd {
	if blocked := view.commentBlocked(); blocked != nil {
		return blocked
	}
	lineRange, ok := view.filePane.CommentRange()
	if !ok {
		return userinterface.Info("move onto a diff line to comment on it")
	}
	view.filePane.VisualEnd()
	in := &github.AddThreadInput{Path: lineRange.Path, Line: lineRange.End, Side: lineRange.Side}
	if lineRange.Start != lineRange.End {
		in.StartLine, in.StartSide = lineRange.Start, lineRange.Side
	}
	return view.compose(target{kind: kindInline, inline: in, head: view.diffHead})
}

// startVisual enters visual-line mode on the diff line under the cursor.
func (view *View) startVisual() tea.Cmd {
	if blocked := view.commentBlocked(); blocked != nil {
		return blocked
	}
	if !view.filePane.VisualStart() {
		return userinterface.Info("move onto a diff line to select lines")
	}
	view.focus = paneRight
	return userinterface.Info("-- VISUAL LINE --  j/k extend, c comment, esc cancel")
}

// visualKey handles a raw key while a line selection is in progress. The
// workspace captures keys then, and esc always leaves.
func (view *View) visualKey(message tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(message, view.bind.VisualExit):
		view.filePane.VisualEnd()
	case key.Matches(message, view.bind.VisualDown):
		return refusal(view.filePane.VisualMove(1))
	case key.Matches(message, view.bind.VisualUp):
		return refusal(view.filePane.VisualMove(-1))
	case key.Matches(message, view.bind.Comment):
		return view.inlineComment()
	}
	return nil
}

func refusal(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return userinterface.Info(err.Error())
}

// commentAtCursor returns the comment that e/D (or the commands) act on and its
// thread. When there is none, or the view cannot change comments now, the
// command to show why.
func (view *View) commentAtCursor(change change) (github.ReviewComment, github.ReviewThread, tea.Cmd) {
	if blocked := view.commentBlocked(); blocked != nil {
		return github.ReviewComment{}, github.ReviewThread{}, blocked
	}
	thread, _, hint := view.focusedThread("") // no thread: pickComment finds nothing
	if hint != nil {
		return github.ReviewComment{}, github.ReviewThread{}, hint
	}
	comment, found := pickComment(thread, change)
	if !found {
		return github.ReviewComment{}, github.ReviewThread{}, userinterface.Info("no comment of yours here")
	}
	return comment, thread, nil
}

// editComment re-reads the viewer's comment at the cursor, then opens the
// editor on its current body, so edits made elsewhere are not overwritten.
func (view *View) editComment() tea.Cmd {
	comment, thread, command := view.commentAtCursor(changeEdit)
	if command != nil {
		return command
	}
	origin, client, timeouts := view.origin(), view.client, view.timeouts
	currentTarget := target{kind: kindEdit, commentIdentifier: comment.Identifier, threadIdentifier: thread.Identifier, label: threadLocation(thread), home: view.reachableHome(thread)}
	view.commentSequence++ // a compose started meanwhile supersedes this read
	sequence := view.commentSequence
	return func() tea.Msg {
		ctx, cancel := timeouts.RequestContext()
		defer cancel()
		fresh, err := freshComment(ctx, client, currentTarget.commentIdentifier, changeEdit)
		currentTarget.initial = fresh.Body
		return commentReadMessage{origin: origin, sequence: sequence, target: currentTarget, err: err}
	}
}

// reachableHome is the position of a thread as `c` would write a comment
// there, or nil when `c` cannot reach it: the thread has no line (outdated and
// file-level threads) or its lines are not in the diff on screen.
func (view *View) reachableHome(thread github.ReviewThread) *github.AddThreadInput {
	if thread.Line == 0 || !view.lineInDiff(thread.Path, thread.DiffSide, thread.Line) || (thread.StartLine > 0 && !view.lineInDiff(thread.Path, thread.StartDiffSide, thread.StartLine)) {
		return nil
	}
	return &github.AddThreadInput{Path: thread.Path, Side: thread.DiffSide, Line: thread.Line, StartLine: thread.StartLine, StartSide: thread.StartDiffSide}
}

// lineInDiff reports whether the diff on screen has line n of path on side.
func (view *View) lineInDiff(path string, side github.DiffSide, count int) bool {
	if view.diff.Parsed == nil {
		return false
	}
	file := view.diff.Parsed.File(path)
	if file == nil {
		return false
	}
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			if (side == github.SideLeft && line.Old == count) || (side != github.SideLeft && line.New == count) {
				return true
			}
		}
	}
	return false
}

// commentRead opens the editor on a freshly read comment, or refuses.
func (view *View) commentRead(message commentReadMessage) tea.Cmd {
	if message.err != nil {
		return view.refuse(message.err, "read comment")
	}
	if blocked := view.commentBlocked(); blocked != nil {
		return blocked
	}
	return view.compose(message.target)
}

// refuse shows why a comment cannot be changed, and refreshes when what is on
// screen turned out to be stale.
func (view *View) refuse(err error, what string) tea.Cmd {
	command := userinterface.Error(commentFailure(err, what))
	if isStale(err) {
		return tea.Batch(command, view.reloadPendingComments())
	}
	return command
}

// deleteComment re-reads the comment at the cursor, then asks before deleting
// it. The question says whether it is pending or published.
func (view *View) deleteComment() tea.Cmd {
	comment, thread, command := view.commentAtCursor(changeDelete)
	if command != nil {
		return command
	}
	origin, identifier, label := view.origin(), comment.Identifier, terminaltext.Clean(threadLocation(thread))
	write := commentWrite{
		origin: origin, client: view.client, timeouts: view.timeouts, identifier: identifier, change: changeDelete,
		ask: func(pending bool) string {
			if pending {
				return "Delete pending comment on " + label + "?"
			}
			return fmt.Sprintf("Delete your published comment on #%d %s?", origin.number, label)
		},
		write: func(ctx context.Context, client github.Client) error {
			return client.DeleteReviewComment(ctx, identifier)
		},
		result: func(err error) tea.Msg {
			result := commentDeletedMessage{origin: origin, err: err}
			if err == nil {
				return result
			}
			return withShownError(result, commentFailure(err, "delete comment"))
		},
	}
	view.writeStarted()
	return func() tea.Msg { return write.run(answer{}) }
}

// commentFailure is the error to show for a failed comment call: the stale
// reason as it is, any other wrapped with what was being done.
func commentFailure(err error, what string) error {
	var staleFailure staleError
	if errors.As(err, &staleFailure) {
		return staleFailure
	}
	return terminaltext.SafeError(fmt.Errorf("%s: %w", what, err))
}

func (view *View) commentDeleted(message commentDeletedMessage) tea.Cmd {
	view.writeDone()
	if isStale(message.err) {
		return view.reloadPendingComments() // the error is already shown
	}
	if message.err != nil {
		return nil
	}
	return tea.Batch(userinterface.Info("Comment deleted"), view.reloadPendingComments())
}

// sendEdit updates a comment of the viewer's. Updating a pending comment runs
// without a prompt; updating a published one asks first. A failed or refused
// edit never throws the text away (see editFailure and staleEdit).
func (view *View) sendEdit(target target, body string) tea.Cmd {
	// What the editor returned is trimmed; so is what was offered to it.
	if body == strings.TrimRight(target.initial, " \t\r\n") {
		delete(view.drafts, target.draftKey())
		return userinterface.Info("comment unchanged")
	}
	origin, client, timeouts, label := view.origin(), view.client, view.timeouts, terminaltext.Clean(target.label)
	write := commentWrite{
		origin: origin, client: client, timeouts: timeouts, identifier: target.commentIdentifier, change: changeEdit,
		ask: func(pending bool) string {
			if pending {
				return ""
			}
			return fmt.Sprintf("Update your published comment on #%d %s?", origin.number, label)
		},
		write: func(ctx context.Context, client github.Client) error {
			_, err := client.UpdateReviewComment(ctx, target.commentIdentifier, body)
			return err
		},
		result: func(err error) tea.Msg {
			result := postedMessage{origin: origin, target: target, body: body, err: err}
			if err == nil {
				return result
			}
			if isStale(err) {
				result.threadLive = threadExists(client, timeouts, origin, target.threadIdentifier)
			}
			return withShownError(result, editFailure(err, target, result.threadLive))
		},
	}
	view.writeStarted()
	return func() tea.Msg { return write.run(answer{}) }
}

// threadExists reports whether a thread is still on the pull request. When the
// threads cannot be listed it says no, which keeps the text where `c` finds it.
func threadExists(client github.Client, timeouts userinterface.Timeouts, origin origin, threadIdentifier string) bool {
	ctx, cancel := timeouts.LoadContext()
	defer cancel()
	threads, err := loadThreads(ctx, client, origin.repository, origin.number)
	return err == nil && slices.ContainsFunc(threads, func(thread github.ReviewThread) bool { return thread.Identifier == threadIdentifier })
}

// homeKey is the draft a refused edit's text goes to: the reply draft of its
// thread while the thread exists, else the draft of a new comment on its lines
// when `c` can reach them, else the top-level draft.
func homeKey(message postedMessage) string {
	switch {
	case message.threadLive:
		return target{kind: kindReply, threadIdentifier: message.target.threadIdentifier}.draftKey()
	case message.target.home != nil:
		return target{kind: kindInline, inline: message.target.home}.draftKey()
	}
	return target{}.draftKey()
}

// editFailure is the error shown for a failed edit, saying where its text is
// kept. A refused edit (a stale comment) goes to the thread's reply draft, or
// when the thread is gone too to the draft for a new comment on its lines (or,
// when `c` cannot reach them, the top-level draft); any other failure leaves the text under the comment's own draft, which `e` finds.
func editFailure(err error, target target, threadLive bool) error {
	var staleFailure staleError
	if !errors.As(err, &staleFailure) {
		return commentFailure(fmt.Errorf("%w; the comment is kept as a draft", err), "update comment")
	}
	switch {
	case threadLive:
		return staleError(string(staleFailure) + "; your text is kept as a reply draft")
	case target.home != nil:
		return staleError(string(staleFailure) + "; your text is kept as a draft for a new comment on " + terminaltext.Clean(lineLabel(target.home)))
	}
	return staleError(string(staleFailure) + "; your text is kept as a draft for a top-level comment")
}

// lineLabel is "path:line" or "path:start-line" for a position.
func lineLabel(in *github.AddThreadInput) string {
	if in.StartLine > 0 && in.StartLine != in.Line {
		return fmt.Sprintf("%s:%d-%d", in.Path, in.StartLine, in.Line)
	}
	return fmt.Sprintf("%s:%d", in.Path, in.Line)
}

// joinDraft appends text to an existing draft, separated by a blank line,
// unless that draft already is the text.
func joinDraft(old, text string) string {
	if old == "" || old == text {
		return text
	}
	return old + "\n\n" + text
}

// staleEdit handles an edit refused because its comment is stale: it
// refreshes, and moves the text to its new home (see homeKey), joined with any
// draft already there. Whether the thread still exists was read after the
// refusal, in the command, because the refreshed list arrives too late and may
// be an older snapshot.
func (view *View) staleEdit(message postedMessage) tea.Cmd {
	if message.target.threadIdentifier != "" {
		key := homeKey(message)
		view.drafts[key] = joinDraft(view.drafts[key], message.body)
		delete(view.drafts, message.target.draftKey())
	}
	return view.reloadPendingComments()
}

// sendInline adds a new inline comment to the pending review (creating the
// review if there is none), anchored to the commit the lines were picked on.
func (view *View) sendInline(target target, body string) tea.Cmd {
	// A diff load in flight is no obstacle: the lines are anchored to target.head,
	// and addPendingThread refuses unless the server is still on it. (The caller
	// checked that the pull request is loaded.)
	origin, client, timeouts := view.origin(), view.client, view.timeouts
	in := *target.inline
	in.Repository, in.Number, in.Body = origin.repository, origin.number, body
	pullRequestIdentifier := view.detail.Identifier
	view.writeStarted()
	return func() tea.Msg {
		err := addPendingThread(timeouts, client, in, pullRequestIdentifier, target.head)
		result := postedMessage{origin: origin, target: target, body: body, err: err}
		if err == nil {
			return result
		}
		return withShownError(result, commentFailure(fmt.Errorf("%w; the comment is kept as a draft", err), "add pending comment"))
	}
}

// addPendingThread adds in to the viewer's pending review, creating it on
// head first when there is none. The comment's line numbers come from the diff
// of commit head, so it is refused when the pull request has moved on or the
// existing pending review is anchored to another (or an unknown) commit.
func addPendingThread(timeouts userinterface.Timeouts, client github.Client, in github.AddThreadInput, pullRequestIdentifier, head string) error {
	statusContext, cancelStatus := timeouts.RequestContext()
	defer cancelStatus()
	status, err := client.GetReviewStatus(statusContext, in.Repository, in.Number)
	if err != nil {
		return fmt.Errorf("check review state: %w", err)
	}
	if status.HeadSHA != head {
		return errors.New("head moved since you loaded the diff; refresh first")
	}
	pending := status.Pending
	switch {
	case pending == nil:
		startContext, cancelStart := timeouts.RequestContext()
		defer cancelStart()
		pendingReview, err := client.StartReview(startContext, github.StartReviewInput{Repository: in.Repository, Number: in.Number, PullRequestIdentifier: pullRequestIdentifier, CommitObjectIdentifier: head})
		if err != nil {
			return fmt.Errorf("start review: %w", userinterface.WriteError(err))
		}
		pending = &pendingReview
	case pending.CommitObjectIdentifier == "":
		return errors.New("your pending review is on an unknown commit; submit or discard it on GitHub first")
	case pending.CommitObjectIdentifier != head:
		return errors.New("your pending review is on an older commit; submit it first, or delete it on GitHub")
	}
	in.PendingReviewIdentifier = pending.Identifier
	addContext, cancelAdd := timeouts.RequestContext()
	defer cancelAdd()
	_, err = client.AddReviewThread(addContext, in)
	return userinterface.WriteError(err)
}
