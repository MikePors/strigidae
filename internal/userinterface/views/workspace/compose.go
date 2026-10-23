package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/editor"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// topLevelDraft is the draft key of the top-level comment; replies are keyed
// by thread ID.
const topLevelDraft = "top-level"

// EditFunc opens the editor on initial and, when it closes, delivers the
// result as the message done builds. The default runs $VISUAL or $EDITOR
// through tea.ExecProcess; tests inject their own.
type EditFunction func(initial string, done func(body string, err error) tea.Msg) tea.Cmd

// WithEditor replaces how comment bodies are edited.
func WithEditor(e EditFunction) Option { return func(v *View) { v.edit = e } }

// targetKind says what a comment being written is for.
type targetKind int

const (
	kindTopLevel targetKind = iota // a comment on the pull request
	kindReply                      // a reply to a review thread
	kindInline                     // a new inline comment in the pending review
	kindEdit                       // a change to one of the viewer's own comments
)

// target is what a comment is written for; kind says which fields apply.
type target struct {
	kind targetKind
	// threadID is the thread replied to (kindReply) or the one the edited
	// comment is in (kindEdit).
	threadIdentifier string
	label            string // "path:line" of the thread, for the questions about it
	// inline (kindInline) is the new comment's position; the repo, number and
	// pending review are filled in when it is sent. head is the commit of the
	// diff the lines were picked on.
	inline *github.AddThreadInput
	head   string
	// commentID (kindEdit) is the comment, pending or published, being
	// edited; initial is its body when the editor was opened, shown there
	// when no draft is kept.
	commentIdentifier string
	initial           string
	// home (kindEdit) is where the thread is in the diff: when the comment's
	// thread is gone, the text is kept as the draft of a new comment there.
	home *github.AddThreadInput
}

func (t target) draftKey() string {
	switch t.kind {
	case kindInline:
		in := t.inline
		return fmt.Sprintf("inline:%s:%s:%d-%d", in.Path, in.Side, in.StartLine, in.Line)
	case kindEdit:
		return "edit:" + t.commentIdentifier
	case kindReply:
		return t.threadIdentifier
	}
	return topLevelDraft
}

// Results of composing carry the same identity as load results. An edit
// result also carries the compose sequence, so starting a new edit makes older
// ones stale; a post result does not, because a later compose does not
// supersede a post.
type (
	editedMessage struct {
		owner      *View
		repository github.Repository
		number     int
		sequence   int
		target     target
		body       string
		err        error
	}
	postedMessage struct {
		origin origin
		target target
		body   string
		err    error
		// threadLive is, for an edit refused as stale, whether the comment's
		// thread still exists (it is gone with its only comment).
		threadLive bool
	}
)

// origin is the identity of the workspace and pull request a result belongs
// to: every view in the stack receives every result, so each ignores the ones
// that are not its own.
type origin struct {
	owner      *View // compared, never dereferenced
	repository github.Repository
	number     int
}

func (v *View) origin() origin { return origin{owner: v, repository: v.repository, number: v.number} }

// owns reports whether a result with origin o was asked for by this view.
func (v *View) owns(o origin) bool { return o == v.origin() }

func (v *View) composeMine(o origin, seq int) bool { return v.owns(o) && seq == v.cseq }

// reply starts a reply to the thread under the diff cursor.
func (v *View) reply() tea.Cmd {
	if busy := v.writeBusy(); busy != nil {
		return busy
	}
	t, ok, hint := v.focusedThread(" to reply")
	if !ok {
		return hint
	}
	return v.compose(target{kind: kindReply, threadIdentifier: t.Identifier, label: threadLocation(t)})
}

// comment starts a top-level comment from the Overview, or an inline one on a
// diff line.
func (v *View) comment() tea.Cmd {
	if v.sel() != 0 {
		return v.inlineComment()
	}
	if busy := v.writeBusy(); busy != nil {
		return busy
	}
	return v.compose(target{})
}

// compose opens the editor, pre-filled with the draft for tgt if there is one.
func (v *View) compose(tgt target) tea.Cmd {
	v.cseq++
	owner, repo, number, seq := v, v.repository, v.number, v.cseq
	initial := v.drafts[tgt.draftKey()]
	if initial == "" {
		initial = tgt.initial
	}
	return v.edit(initial, func(body string, err error) tea.Msg {
		return editedMessage{owner: owner, repository: repo, number: number, sequence: seq, target: tgt, body: body, err: err}
	})
}

// edited asks for confirmation once the editor closed with a body.
func (v *View) edited(msg editedMessage) tea.Cmd {
	body := strings.TrimRight(msg.body, " \t\r\n")
	if msg.err != nil {
		// Whatever was typed is kept, so the next try starts from it.
		if strings.TrimSpace(body) != "" {
			v.drafts[msg.target.draftKey()] = body
		}
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("edit comment: %w", msg.err)))
	}
	if strings.TrimSpace(body) == "" {
		return userinterface.Error(errors.New("empty comment, nothing posted (did the editor fail?)"))
	}
	// Kept until the post succeeds, so declining or a failure loses nothing.
	v.drafts[msg.target.draftKey()] = body
	// A pending comment is a private draft, so it is sent without asking;
	// sendEdit asks before changing a published one.
	switch msg.target.kind {
	case kindInline, kindEdit:
		// The diff being reloaded does not matter now: the head check in the
		// send covers it. The text stays a draft if the send is refused.
		env := v.reviewEnvironment()
		env.diffReady = true
		if blocked := v.review.blocked(env); blocked != nil {
			return blocked
		}
		if msg.target.kind == kindInline {
			return v.sendInline(msg.target, body)
		}
		return v.sendEdit(msg.target, body)
	}
	return userinterface.Confirm(v.question(msg.target, body), v.agreed(v.post(msg.target, body)))
}

// question is the confirmation text: where the text goes and its first line.
func (v *View) question(tgt target, body string) string {
	prefix := "Post comment?"
	if tgt.kind == kindReply {
		prefix = "Post reply to " + terminaltext.Clean(tgt.label) + "?"
	}
	first := firstLine(body)
	// Leave room for the breadcrumbs, the quotes and " y/N".
	room := max(10, v.width-ansi.StringWidth(prefix)-ansi.StringWidth(v.Title())-14)
	return prefix + ` "` + ansi.Truncate(first, room, "…") + `"`
}

func firstLine(body string) string {
	for l := range strings.SplitSeq(terminaltext.CleanBlock(body), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// post is the command that runs when the user confirms. On failure it also
// returns the error itself, so it shows even if the view was left meanwhile.
func (v *View) post(tgt target, body string) tea.Cmd {
	client, o := v.client, v.origin()
	return func() tea.Msg {
		var err error
		if tgt.kind == kindReply {
			_, err = client.ReplyToThread(context.Background(), tgt.threadIdentifier, body)
		} else {
			_, err = client.AddComment(context.Background(), o.repository, o.number, body)
		}
		res := postedMessage{origin: o, target: tgt, body: body, err: err}
		if err == nil {
			return res
		}
		what := "comment"
		if tgt.kind == kindReply {
			what = "reply"
		}
		shown := userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("post %s: %w", what, err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

func (v *View) posted(msg postedMessage) tea.Cmd {
	v.writeDone()
	if isStale(msg.err) {
		return v.staleEdit(msg)
	}
	if msg.err != nil {
		return nil // the draft stays; the command already reported the error
	}
	// A newer draft for the same target must survive.
	if v.drafts[msg.target.draftKey()] == msg.body {
		delete(v.drafts, msg.target.draftKey())
	}
	switch msg.target.kind {
	case kindReply:
		return tea.Batch(userinterface.Info("Reply posted"), v.reloadThreads())
	case kindInline:
		return tea.Batch(userinterface.Info("Pending comment added"), v.reloadPendingComments())
	case kindEdit:
		return tea.Batch(userinterface.Info("Comment updated"), v.reloadPendingComments())
	}
	return tea.Batch(userinterface.Info("Comment posted"), v.reloadPullRequest())
}

// threadLocation is "path:line" for a thread.
func threadLocation(t github.ReviewThread) string {
	line := t.Line
	if line == 0 {
		line = t.OriginalLine
	}
	if line == 0 {
		return t.Path
	}
	return t.Path + ":" + strconv.Itoa(line)
}

// execEditor is the default EditFunc: a temporary file opened in the user's
// editor, with the TUI suspended meanwhile.
func execEditor(initial string, done func(string, error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		s, err := editor.New(initial, os.Getenv)
		if err != nil {
			return done("", err)
		}
		return tea.ExecProcess(s.Command(), func(runErr error) tea.Msg {
			body, readErr := s.Result()
			return done(body, errors.Join(runErr, readErr))
		})()
	}
}
