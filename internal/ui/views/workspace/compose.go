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
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// topLevelDraft is the draft key of the top-level comment; replies are keyed
// by thread ID.
const topLevelDraft = "top-level"

// EditFunc opens the editor on initial and, when it closes, delivers the
// result as the message done builds. The default runs $VISUAL or $EDITOR
// through tea.ExecProcess; tests inject their own.
type EditFunc func(initial string, done func(body string, err error) tea.Msg) tea.Cmd

// WithEditor replaces how comment bodies are edited.
func WithEditor(e EditFunc) Option { return func(v *View) { v.edit = e } }

// target is what a comment is written for.
type target struct {
	threadID string // empty for a top-level comment
	label    string // "path:line" of the thread, for the confirmation
}

func (t target) draftKey() string {
	if t.threadID == "" {
		return topLevelDraft
	}
	return t.threadID
}

// Results of composing carry the same identity as load results. An edit
// result also carries the compose sequence, so starting a new edit makes older
// ones stale; a post result does not, because a later compose does not
// supersede a post.
type (
	editedMsg struct {
		owner  *View
		repo   github.Repo
		number int
		seq    int
		tgt    target
		body   string
		err    error
	}
	postedMsg struct {
		owner  *View
		repo   github.Repo
		number int
		tgt    target
		body   string
		err    error
	}
)

func (v *View) composeMine(owner *View, repo github.Repo, number, seq int) bool {
	return owner == v && repo == v.repo && number == v.number && seq == v.cseq
}

// reply starts a reply to the thread under the diff cursor.
func (v *View) reply() tea.Cmd {
	if v.sel() == 0 || v.focus != paneRight {
		return nil
	}
	switch ts := v.filePane.FocusedThreads(); len(ts) {
	case 0:
		return nil
	case 1:
		return v.compose(target{threadID: ts[0].ID, label: threadLocation(ts[0])})
	default:
		return ui.Info("several threads on this line: move onto one to reply")
	}
}

// comment starts a top-level comment from the Overview.
func (v *View) comment() tea.Cmd {
	if v.sel() != 0 {
		return nil
	}
	return v.compose(target{})
}

// compose opens the editor, pre-filled with the draft for tgt if there is one.
func (v *View) compose(tgt target) tea.Cmd {
	v.cseq++
	owner, repo, number, seq := v, v.repo, v.number, v.cseq
	return v.edit(v.drafts[tgt.draftKey()], func(body string, err error) tea.Msg {
		return editedMsg{owner: owner, repo: repo, number: number, seq: seq, tgt: tgt, body: body, err: err}
	})
}

// edited asks for confirmation once the editor closed with a body.
func (v *View) edited(msg editedMsg) tea.Cmd {
	body := strings.TrimRight(msg.body, " \t\r\n")
	if msg.err != nil {
		// Whatever was typed is kept, so the next try starts from it.
		if strings.TrimSpace(body) != "" {
			v.drafts[msg.tgt.draftKey()] = body
		}
		return ui.Error(termtext.SafeError(fmt.Errorf("edit comment: %w", msg.err)))
	}
	if strings.TrimSpace(body) == "" {
		return ui.Error(errors.New("empty comment, nothing posted (did the editor fail?)"))
	}
	// Kept until the post succeeds, so declining or a failure loses nothing.
	v.drafts[msg.tgt.draftKey()] = body
	return ui.Confirm(v.question(msg.tgt, body), v.post(msg.tgt, body))
}

// question is the confirmation text: where the text goes and its first line.
func (v *View) question(tgt target, body string) string {
	prefix := "Post comment?"
	if tgt.threadID != "" {
		prefix = "Post reply to " + termtext.Clean(tgt.label) + "?"
	}
	first := firstLine(body)
	// Leave room for the breadcrumbs, the quotes and " y/N".
	room := max(10, v.width-ansi.StringWidth(prefix)-ansi.StringWidth(v.Title())-14)
	return prefix + ` "` + ansi.Truncate(first, room, "…") + `"`
}

func firstLine(body string) string {
	for l := range strings.SplitSeq(termtext.CleanBlock(body), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// post is the command that runs when the user confirms. On failure it also
// returns the error itself, so it shows even if the view was left meanwhile.
func (v *View) post(tgt target, body string) tea.Cmd {
	client, repo, number, owner := v.client, v.repo, v.number, v
	return func() tea.Msg {
		var err error
		if tgt.threadID != "" {
			_, err = client.ReplyToThread(context.Background(), tgt.threadID, body)
		} else {
			_, err = client.AddComment(context.Background(), repo, number, body)
		}
		res := postedMsg{owner: owner, repo: repo, number: number, tgt: tgt, body: body, err: err}
		if err == nil {
			return res
		}
		what := "comment"
		if tgt.threadID != "" {
			what = "reply"
		}
		shown := ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("post %s: %w", what, err))}
		return tea.BatchMsg{func() tea.Msg { return res }, func() tea.Msg { return shown }}
	}
}

func (v *View) posted(msg postedMsg) tea.Cmd {
	if msg.err != nil {
		return nil // the draft stays; the command already reported the error
	}
	what, reload := "Comment", v.reloadPR
	if msg.tgt.threadID != "" {
		what, reload = "Reply", v.reloadThreads
	}
	// A newer draft for the same target must survive.
	if v.drafts[msg.tgt.draftKey()] == msg.body {
		delete(v.drafts, msg.tgt.draftKey())
	}
	return tea.Batch(ui.Info(what+" posted"), reload())
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
		return tea.ExecProcess(s.Cmd(), func(runErr error) tea.Msg {
			body, readErr := s.Result()
			return done(body, errors.Join(runErr, readErr))
		})()
	}
}
