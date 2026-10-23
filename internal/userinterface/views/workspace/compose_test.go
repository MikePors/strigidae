package workspace

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// composer is a harness whose editor is a stub: it records the text it was
// opened on and answers with body/err.
type composer struct {
	*harness
	body     string
	err      error
	openedOn []string
}

func newComposer(t *testing.T) *composer {
	c := &composer{harness: threadHarness(t, 110, 40)}
	c.v.edit = func(initial string, done func(string, error) tea.Msg) tea.Cmd {
		c.openedOn = append(c.openedOn, initial)
		return func() tea.Msg { return done(c.body, c.err) }
	}
	return c
}

// lastConfirm returns the most recent confirmation asked, and forgets it.
func (c *composer) lastConfirm() (userinterface.ConfirmMsg, bool) {
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if m, ok := c.msgs[i].(userinterface.ConfirmMsg); ok {
			c.msgs = c.msgs[:i]
			return m, true
		}
	}
	return userinterface.ConfirmMsg{}, false
}

func (c *composer) infos() (out []string) {
	for _, m := range c.msgs {
		if i, ok := m.(userinterface.InfoMsg); ok {
			out = append(out, i.Text)
		}
	}
	return out
}

// onThread selects the client.go diff and moves to the resolved thread on line 12.
func (c *composer) onThread() {
	c.selectFile()
	c.press(")")
}

func TestReplyAsksThenPosts(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.body = "Thanks, fixed.\nmore\n"
	c.press("r")
	if diff := cmp.Diff([]string{""}, c.openedOn); diff != "" {
		t.Fatalf("editor opened on (-want +got):\n%s", diff)
	}
	conf, ok := c.lastConfirm()
	if !ok {
		t.Fatal("no confirmation asked")
	}
	if want := `Post reply to internal/sync/client.go:12? "Thanks, fixed."`; conf.Text != want {
		t.Errorf("text = %q, want %q", conf.Text, want)
	}
	if len(c.client.Replies()) != 0 {
		t.Fatal("posted before the answer")
	}
	c.run(conf.OnYes)
	want := []fake.Reply{{ThreadID: "t-resolved", Body: "Thanks, fixed.\nmore"}}
	if diff := cmp.Diff(want, c.client.Replies()); diff != "" {
		t.Errorf("replies (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Reply posted"}, c.infos()); diff != "" {
		t.Errorf("info (-want +got):\n%s", diff)
	}
	if len(c.v.drafts) != 0 {
		t.Errorf("draft kept after success: %v", c.v.drafts)
	}
	var n int
	for _, th := range c.v.threads {
		if th.ID == "t-resolved" {
			n = len(th.Comments)
		}
	}
	if n != 2 {
		t.Errorf("threads not reloaded: resolved thread has %d comments", n)
	}
}

func TestDeclinedConfirmationKeepsDraft(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.body = "my draft"
	c.press("r")
	if _, ok := c.lastConfirm(); !ok {
		t.Fatal("no confirmation")
	}
	// Declining is the shell dropping OnYes: nothing runs, nothing is posted.
	if len(c.client.Replies()) != 0 {
		t.Error("something was posted")
	}
	c.press("r")
	if diff := cmp.Diff([]string{"", "my draft"}, c.openedOn); diff != "" {
		t.Errorf("second editor not pre-filled (-want +got):\n%s", diff)
	}
}

func TestDraftsAreKeyedByTarget(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.body = "reply draft"
	c.press("r")
	c.press("h", "g g") // back to the Files pane, Overview
	c.body = "top draft"
	c.press("c")
	c.press("c")
	if diff := cmp.Diff([]string{"", "", "top draft"}, c.openedOn); diff != "" {
		t.Errorf("editor pre-fill (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(map[string]string{"t-resolved": "reply draft", "top-level": "top draft"}, c.v.drafts); diff != "" {
		t.Errorf("drafts (-want +got):\n%s", diff)
	}
}

func TestEmptyBodyPostsNothing(t *testing.T) {
	for _, body := range []string{"", "  \n\t\n"} {
		c := newComposer(t)
		c.onThread()
		c.body = body
		c.press("r")
		if _, ok := c.lastConfirm(); ok {
			t.Errorf("%q: asked to confirm an empty body", body)
		}
		err := c.lastErr()
		if err == nil || !strings.Contains(err.Error(), "empty comment") {
			t.Errorf("%q: error = %v", body, err)
		}
		if len(c.v.drafts) != 0 || len(c.client.Replies()) != 0 {
			t.Errorf("%q: drafts %v, replies %v", body, c.v.drafts, c.client.Replies())
		}
	}
}

func TestEditorFailureShowsError(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.err = errors.New("exit status 1")
	c.body = "half written"
	c.press("r")
	if c.lastErr() == nil {
		t.Error("no error shown")
	}
	if _, ok := c.lastConfirm(); ok {
		t.Error("asked to confirm after a failed editor")
	}
	if c.v.drafts["t-resolved"] != "half written" {
		t.Errorf("typed text not kept as the draft: %v", c.v.drafts)
	}
	c.err, c.body = nil, ""
	c.press("r")
	if got := c.openedOn[len(c.openedOn)-1]; got != "half written" {
		t.Errorf("next editor opened on %q", got)
	}
}

func TestFailedPostKeepsDraft(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.client.SetError(fake.MethodReplyToThread, errors.New("boom\x1b[31m"))
	c.body = "important"
	c.press("r")
	conf, _ := c.lastConfirm()
	c.run(conf.OnYes)
	err := c.lastErr()
	if err == nil || !strings.Contains(err.Error(), "post reply: boom") || strings.ContainsRune(err.Error(), 0x1b) {
		t.Errorf("error = %v", err)
	}
	if c.v.drafts["t-resolved"] != "important" {
		t.Errorf("draft lost: %v", c.v.drafts)
	}
	c.press("r")
	if got := c.openedOn[len(c.openedOn)-1]; got != "important" {
		t.Errorf("next editor opened on %q", got)
	}
}

func TestStaleComposeResultsIgnored(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.body = "x"
	c.press("r")
	stale := editedMsg{owner: c.v, repo: testRepo, number: 7, seq: c.v.cseq - 1, tgt: target{kind: kindReply, threadID: "t-resolved"}, body: "late"}
	c.msgs = nil
	c.send(stale)
	if _, ok := c.lastConfirm(); ok {
		t.Error("stale edit result asked for confirmation")
	}
	other := stale
	other.owner, other.seq = &View{}, c.v.cseq
	c.send(other)
	c.send(editedMsg{owner: c.v, repo: github.Repo{Owner: "x", Name: "y"}, number: 7, seq: c.v.cseq, body: "late"})
	c.send(editedMsg{owner: c.v, repo: testRepo, number: 8, seq: c.v.cseq, body: "late"})
	if _, ok := c.lastConfirm(); ok || c.v.drafts["t-resolved"] == "late" {
		t.Error("foreign edit result was accepted")
	}
}

func TestPostResultAfterNewComposeIsHandled(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.body = "first"
	c.press("r")
	conf, _ := c.lastConfirm()
	// The user starts another compose before the post returns.
	c.body = "second"
	before := c.v.cseq
	c.press("h", "g g", "c")
	if c.v.cseq == before {
		t.Fatal("setup: no new compose started")
	}
	c.drafts(map[string]string{"t-resolved": "first"})
	c.msgs = nil
	c.run(conf.OnYes)
	if diff := cmp.Diff([]string{"Reply posted"}, c.infos()); diff != "" {
		t.Errorf("info (-want +got):\n%s", diff)
	}
	if _, ok := c.v.drafts["t-resolved"]; ok {
		t.Error("posted draft kept")
	}

	// A newer draft for the same target survives the older post.
	c.drafts(map[string]string{"t-resolved": "newer"})
	c.send(postedMsg{origin: c.v.origin(), tgt: target{kind: kindReply, threadID: "t-resolved"}, body: "first"})
	if c.v.drafts["t-resolved"] != "newer" {
		t.Errorf("newer draft deleted: %v", c.v.drafts)
	}

	// A failure is reported by the command itself, and keeps the draft.
	c.client.SetError(fake.MethodReplyToThread, errors.New("boom"))
	c.drafts(map[string]string{"t-resolved": "first"})
	c.msgs = nil
	c.run(c.v.post(target{kind: kindReply, threadID: "t-resolved"}, "first"))
	if err := c.lastErr(); err == nil || !strings.Contains(err.Error(), "post reply: boom") {
		t.Errorf("error = %v", err)
	}
	if c.v.drafts["t-resolved"] != "first" {
		t.Error("draft lost")
	}
}

func (c *composer) drafts(d map[string]string) { c.v.drafts = d }

func TestReplyWithZeroOneAndSeveralThreads(t *testing.T) {
	c := newComposer(t)
	two := []github.ReviewThread{
		{ID: "a", Path: "internal/sync/client.go", Line: 14, DiffSide: github.SideRight, Comments: []github.ReviewComment{tc("bob", "one", time.Hour)}},
		{ID: "b", Path: "internal/sync/client.go", Line: 14, DiffSide: github.SideRight, Comments: []github.ReviewComment{tc("eve", "two", time.Hour)}},
		{ID: "c", Path: "internal/sync/client.go", Line: 12, DiffSide: github.SideRight, Comments: []github.ReviewComment{tc("dan", "three", time.Hour)}},
	}
	c.client.SetReviewThreads(testRepo, 7, two)
	c.run(c.v.load())
	c.selectFile()
	c.body = "hi"

	c.press("g g") // a hunk header: no thread
	c.press("r")
	if len(c.openedOn) != 0 || c.lastErr() != nil || len(c.msgs) != 0 {
		t.Errorf("zero threads: opened %v, msgs %v", c.openedOn, c.msgs)
	}

	c.press(")") // line 12, one thread
	c.press("r")
	if len(c.openedOn) != 1 {
		t.Fatalf("one thread: editor opened %d times", len(c.openedOn))
	}
	conf, _ := c.lastConfirm()
	c.run(conf.OnYes)
	if r := c.client.Replies(); len(r) != 1 || r[0].ThreadID != "c" {
		t.Errorf("replies = %v", r)
	}

	c.msgs = nil
	for range 12 { // walk the diff until the cursor is on line 14, two threads
		c.press("j")
		if len(c.v.filePane.FocusedThreads()) == 2 {
			break
		}
	}
	if n := len(c.v.filePane.FocusedThreads()); n != 2 {
		t.Fatalf("setup: %d threads at the cursor", n)
	}
	c.press("r")
	if len(c.openedOn) != 1 {
		t.Error("editor opened with several threads")
	}
	if diff := cmp.Diff([]string{"several threads on this line: move onto one to reply"}, c.infos()); diff != "" {
		t.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestReplyInOverviewDoesNothing(t *testing.T) {
	c := newComposer(t)
	c.body = "x"
	c.press("r") // Overview selected
	if len(c.openedOn) != 0 || len(c.msgs) != 0 {
		t.Errorf("opened %v, msgs %v", c.openedOn, c.msgs)
	}
}

func TestReplyWorksFromTheFilesPane(t *testing.T) {
	c := newComposer(t)
	c.onThread()
	c.body = "from the files pane"
	c.press("h", "r") // the diff cursor stays on the thread
	if c.v.focus != paneFiles || len(c.openedOn) != 1 {
		t.Fatalf("focus %v, editor opened %d times", c.v.focus, len(c.openedOn))
	}
	conf, ok := c.lastConfirm()
	if !ok || !strings.HasPrefix(conf.Text, "Post reply to internal/sync/client.go:12?") {
		t.Errorf("confirmation = %q (%v)", conf.Text, ok)
	}
}

func TestTopLevelCommentIsPostedFromTheOverview(t *testing.T) {
	c := newComposer(t)
	c.body = "LGTM, one question\nbelow"
	c.press("l", "c")
	conf, ok := c.lastConfirm()
	if !ok || conf.Text != `Post comment? "LGTM, one question"` {
		t.Fatalf("confirmation = %q (%v)", conf.Text, ok)
	}
	c.run(conf.OnYes)
	want := []fake.Comment{{Repo: testRepo, Number: 7, Body: "LGTM, one question\nbelow"}}
	if diff := cmp.Diff(want, c.client.Comments()); diff != "" {
		t.Errorf("comments (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Comment posted"}, c.infos()); diff != "" {
		t.Errorf("info (-want +got):\n%s", diff)
	}
	c.press("G")
	if out := ansi.Strip(c.render()); !strings.Contains(out, "LGTM, one question") {
		t.Errorf("conversation not reloaded:\n%s", out)
	}

	c.msgs, c.openedOn = nil, nil
	c.selectFile()
	c.press("c")
	// On a hunk header there is no line to comment on (inline comments have
	// their own tests in inline_test.go).
	if len(c.openedOn) != 0 || len(c.infos()) != 1 || !strings.Contains(c.infos()[0], "diff line") {
		t.Errorf("c on a hunk header: %v %v", c.openedOn, c.msgs)
	}
}

func TestConfirmationTextIsCleanAndFits(t *testing.T) {
	c := newComposer(t)
	c.body = hostile + strings.Repeat(" long", 60)
	c.press("c")
	conf, _ := c.lastConfirm()
	if strings.ContainsFunc(conf.Text, func(r rune) bool { return r < 0x20 || (r >= 0x7f && r < 0xa0) }) {
		t.Errorf("control characters in %q", conf.Text)
	}
	if w := ansi.StringWidth(conf.Text); w > c.v.width {
		t.Errorf("text is %d wide, view is %d", w, c.v.width)
	}
}

func TestReplyAndCommentAreInHelp(t *testing.T) {
	h := loaded(t, 100, 30)
	got := map[string]string{}
	for _, g := range h.v.FullHelp() {
		for _, b := range g {
			if k := b.Keys(); len(k) == 1 && (k[0] == "r" || k[0] == "c") {
				got[k[0]] = b.Help().Desc
			}
		}
	}
	if len(got) != 2 || got["r"] == "" || got["c"] == "" {
		t.Errorf("help = %v", got)
	}
}
