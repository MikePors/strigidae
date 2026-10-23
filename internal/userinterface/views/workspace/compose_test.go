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

func newComposer(test *testing.T) *composer {
	testComposer := &composer{harness: threadHarness(test, 110, 40)}
	testComposer.view.edit = func(initial string, done func(string, error) tea.Msg) tea.Cmd {
		testComposer.openedOn = append(testComposer.openedOn, initial)
		return func() tea.Msg { return done(testComposer.body, testComposer.err) }
	}
	return testComposer
}

// lastConfirm returns the most recent confirmation asked, and forgets it.
func (composer *composer) lastConfirm() (userinterface.ConfirmMessage, bool) {
	for i := len(composer.messages) - 1; i >= 0; i-- {
		if message, ok := composer.messages[i].(userinterface.ConfirmMessage); ok {
			composer.messages = composer.messages[:i]
			return message, true
		}
	}
	return userinterface.ConfirmMessage{}, false
}

func (composer *composer) infoMessages() (out []string) {
	for _, message := range composer.messages {
		if i, ok := message.(userinterface.InfoMessage); ok {
			out = append(out, i.Text)
		}
	}
	return out
}

// onThread selects the client.go diff and moves to the resolved thread on line 12.
func (composer *composer) onThread() {
	composer.selectFile()
	composer.press(")")
}

func TestReplyAsksThenPosts(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.body = "Thanks, fixed.\nmore\n"
	composer.press("r")
	if diff := cmp.Diff([]string{""}, composer.openedOn); diff != "" {
		test.Fatalf("editor opened on (-want +got):\n%s", diff)
	}
	configuration, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no confirmation asked")
	}
	if want := `Post reply to internal/sync/client.go:12? "Thanks, fixed."`; configuration.Text != want {
		test.Errorf("text = %q, want %q", configuration.Text, want)
	}
	if len(composer.client.Replies()) != 0 {
		test.Fatal("posted before the answer")
	}
	composer.run(configuration.OnYes)
	want := []fake.Reply{{ThreadIdentifier: "t-resolved", Body: "Thanks, fixed.\nmore"}}
	if diff := cmp.Diff(want, composer.client.Replies()); diff != "" {
		test.Errorf("replies (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Reply posted"}, composer.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if len(composer.view.drafts) != 0 {
		test.Errorf("draft kept after success: %v", composer.view.drafts)
	}
	var count int
	for _, thread := range composer.view.threads {
		if thread.Identifier == "t-resolved" {
			count = len(thread.Comments)
		}
	}
	if count != 2 {
		test.Errorf("threads not reloaded: resolved thread has %d comments", count)
	}
}

func TestDeclinedConfirmationKeepsDraft(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.body = "my draft"
	composer.press("r")
	if _, ok := composer.lastConfirm(); !ok {
		test.Fatal("no confirmation")
	}
	// Declining is the shell dropping OnYes: nothing runs, nothing is posted.
	if len(composer.client.Replies()) != 0 {
		test.Error("something was posted")
	}
	composer.press("r")
	if diff := cmp.Diff([]string{"", "my draft"}, composer.openedOn); diff != "" {
		test.Errorf("second editor not pre-filled (-want +got):\n%s", diff)
	}
}

func TestDraftsAreKeyedByTarget(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.body = "reply draft"
	composer.press("r")
	composer.press("h", "g g") // back to the Files pane, Overview
	composer.body = "top draft"
	composer.press("c")
	composer.press("c")
	if diff := cmp.Diff([]string{"", "", "top draft"}, composer.openedOn); diff != "" {
		test.Errorf("editor pre-fill (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(map[string]string{"t-resolved": "reply draft", "top-level": "top draft"}, composer.view.drafts); diff != "" {
		test.Errorf("drafts (-want +got):\n%s", diff)
	}
}

func TestEmptyBodyPostsNothing(test *testing.T) {
	for _, body := range []string{"", "  \n\t\n"} {
		composer := newComposer(test)
		composer.onThread()
		composer.body = body
		composer.press("r")
		if _, ok := composer.lastConfirm(); ok {
			test.Errorf("%q: asked to confirm an empty body", body)
		}
		err := composer.lastErr()
		if err == nil || !strings.Contains(err.Error(), "empty comment") {
			test.Errorf("%q: error = %v", body, err)
		}
		if len(composer.view.drafts) != 0 || len(composer.client.Replies()) != 0 {
			test.Errorf("%q: drafts %v, replies %v", body, composer.view.drafts, composer.client.Replies())
		}
	}
}

func TestEditorFailureShowsError(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.err = errors.New("exit status 1")
	composer.body = "half written"
	composer.press("r")
	if composer.lastErr() == nil {
		test.Error("no error shown")
	}
	if _, ok := composer.lastConfirm(); ok {
		test.Error("asked to confirm after a failed editor")
	}
	if composer.view.drafts["t-resolved"] != "half written" {
		test.Errorf("typed text not kept as the draft: %v", composer.view.drafts)
	}
	composer.err, composer.body = nil, ""
	composer.press("r")
	if got := composer.openedOn[len(composer.openedOn)-1]; got != "half written" {
		test.Errorf("next editor opened on %q", got)
	}
}

func TestFailedPostKeepsDraft(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.client.SetError(fake.MethodReplyToThread, errors.New("boom\x1b[31m"))
	composer.body = "important"
	composer.press("r")
	configuration, _ := composer.lastConfirm()
	composer.run(configuration.OnYes)
	err := composer.lastErr()
	if err == nil || !strings.Contains(err.Error(), "post reply: boom") || strings.ContainsRune(err.Error(), 0x1b) {
		test.Errorf("error = %v", err)
	}
	if composer.view.drafts["t-resolved"] != "important" {
		test.Errorf("draft lost: %v", composer.view.drafts)
	}
	composer.press("r")
	if got := composer.openedOn[len(composer.openedOn)-1]; got != "important" {
		test.Errorf("next editor opened on %q", got)
	}
}

func TestStaleComposeResultsIgnored(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.body = "x"
	composer.press("r")
	stale := editedMessage{owner: composer.view, repository: testRepository, number: 7, sequence: composer.view.commentSequence - 1, target: target{kind: kindReply, threadIdentifier: "t-resolved"}, body: "late"}
	composer.messages = nil
	composer.send(stale)
	if _, ok := composer.lastConfirm(); ok {
		test.Error("stale edit result asked for confirmation")
	}
	other := stale
	other.owner, other.sequence = &View{}, composer.view.commentSequence
	composer.send(other)
	composer.send(editedMessage{owner: composer.view, repository: github.Repository{Owner: "x", Name: "y"}, number: 7, sequence: composer.view.commentSequence, body: "late"})
	composer.send(editedMessage{owner: composer.view, repository: testRepository, number: 8, sequence: composer.view.commentSequence, body: "late"})
	if _, ok := composer.lastConfirm(); ok || composer.view.drafts["t-resolved"] == "late" {
		test.Error("foreign edit result was accepted")
	}
}

func TestPostResultAfterNewComposeIsHandled(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.body = "first"
	composer.press("r")
	configuration, _ := composer.lastConfirm()
	// The user starts another compose before the post returns.
	composer.body = "second"
	before := composer.view.commentSequence
	composer.press("h", "g g", "c")
	if composer.view.commentSequence == before {
		test.Fatal("setup: no new compose started")
	}
	composer.drafts(map[string]string{"t-resolved": "first"})
	composer.messages = nil
	composer.run(configuration.OnYes)
	if diff := cmp.Diff([]string{"Reply posted"}, composer.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if _, ok := composer.view.drafts["t-resolved"]; ok {
		test.Error("posted draft kept")
	}

	// A newer draft for the same target survives the older post.
	composer.drafts(map[string]string{"t-resolved": "newer"})
	composer.send(postedMessage{origin: composer.view.origin(), target: target{kind: kindReply, threadIdentifier: "t-resolved"}, body: "first"})
	if composer.view.drafts["t-resolved"] != "newer" {
		test.Errorf("newer draft deleted: %v", composer.view.drafts)
	}

	// A failure is reported by the command itself, and keeps the draft.
	composer.client.SetError(fake.MethodReplyToThread, errors.New("boom"))
	composer.drafts(map[string]string{"t-resolved": "first"})
	composer.messages = nil
	composer.run(composer.view.post(target{kind: kindReply, threadIdentifier: "t-resolved"}, "first"))
	if err := composer.lastErr(); err == nil || !strings.Contains(err.Error(), "post reply: boom") {
		test.Errorf("error = %v", err)
	}
	if composer.view.drafts["t-resolved"] != "first" {
		test.Error("draft lost")
	}
}

func (composer *composer) drafts(data map[string]string) { composer.view.drafts = data }

func TestReplyWithZeroOneAndSeveralThreads(test *testing.T) {
	composer := newComposer(test)
	two := []github.ReviewThread{
		{Identifier: "a", Path: "internal/sync/client.go", Line: 14, DiffSide: github.SideRight, Comments: []github.ReviewComment{newComment("bob", "one", time.Hour)}},
		{Identifier: "b", Path: "internal/sync/client.go", Line: 14, DiffSide: github.SideRight, Comments: []github.ReviewComment{newComment("eve", "two", time.Hour)}},
		{Identifier: "c", Path: "internal/sync/client.go", Line: 12, DiffSide: github.SideRight, Comments: []github.ReviewComment{newComment("dan", "three", time.Hour)}},
	}
	composer.client.SetReviewThreads(testRepository, 7, two)
	composer.run(composer.view.load())
	composer.selectFile()
	composer.body = "hi"

	composer.press("g g") // a hunk header: no thread
	composer.press("r")
	if len(composer.openedOn) != 0 || composer.lastErr() != nil || len(composer.messages) != 0 {
		test.Errorf("zero threads: opened %v, msgs %v", composer.openedOn, composer.messages)
	}

	composer.press(")") // line 12, one thread
	composer.press("r")
	if len(composer.openedOn) != 1 {
		test.Fatalf("one thread: editor opened %d times", len(composer.openedOn))
	}
	configuration, _ := composer.lastConfirm()
	composer.run(configuration.OnYes)
	if review := composer.client.Replies(); len(review) != 1 || review[0].ThreadIdentifier != "c" {
		test.Errorf("replies = %v", review)
	}

	composer.messages = nil
	for range 12 { // walk the diff until the cursor is on line 14, two threads
		composer.press("j")
		if len(composer.view.filePane.FocusedThreads()) == 2 {
			break
		}
	}
	if count := len(composer.view.filePane.FocusedThreads()); count != 2 {
		test.Fatalf("setup: %d threads at the cursor", count)
	}
	composer.press("r")
	if len(composer.openedOn) != 1 {
		test.Error("editor opened with several threads")
	}
	if diff := cmp.Diff([]string{"several threads on this line: move onto one to reply"}, composer.infoMessages()); diff != "" {
		test.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestReplyInOverviewDoesNothing(test *testing.T) {
	composer := newComposer(test)
	composer.body = "x"
	composer.press("r") // Overview selected
	if len(composer.openedOn) != 0 || len(composer.messages) != 0 {
		test.Errorf("opened %v, msgs %v", composer.openedOn, composer.messages)
	}
}

func TestReplyWorksFromTheFilesPane(test *testing.T) {
	composer := newComposer(test)
	composer.onThread()
	composer.body = "from the files pane"
	composer.press("h", "r") // the diff cursor stays on the thread
	if composer.view.focus != paneFiles || len(composer.openedOn) != 1 {
		test.Fatalf("focus %v, editor opened %d times", composer.view.focus, len(composer.openedOn))
	}
	configuration, ok := composer.lastConfirm()
	if !ok || !strings.HasPrefix(configuration.Text, "Post reply to internal/sync/client.go:12?") {
		test.Errorf("confirmation = %q (%v)", configuration.Text, ok)
	}
}

func TestTopLevelCommentIsPostedFromTheOverview(test *testing.T) {
	composer := newComposer(test)
	composer.body = "LGTM, one question\nbelow"
	composer.press("l", "c")
	configuration, ok := composer.lastConfirm()
	if !ok || configuration.Text != `Post comment? "LGTM, one question"` {
		test.Fatalf("confirmation = %q (%v)", configuration.Text, ok)
	}
	composer.run(configuration.OnYes)
	want := []fake.Comment{{Repository: testRepository, Number: 7, Body: "LGTM, one question\nbelow"}}
	if diff := cmp.Diff(want, composer.client.Comments()); diff != "" {
		test.Errorf("comments (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Comment posted"}, composer.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	composer.press("G")
	if out := ansi.Strip(composer.render()); !strings.Contains(out, "LGTM, one question") {
		test.Errorf("conversation not reloaded:\n%s", out)
	}

	composer.messages, composer.openedOn = nil, nil
	composer.selectFile()
	composer.press("c")
	// On a hunk header there is no line to comment on (inline comments have
	// their own tests in inline_test.go).
	if len(composer.openedOn) != 0 || len(composer.infoMessages()) != 1 || !strings.Contains(composer.infoMessages()[0], "diff line") {
		test.Errorf("c on a hunk header: %v %v", composer.openedOn, composer.messages)
	}
}

func TestConfirmationTextIsCleanAndFits(test *testing.T) {
	composer := newComposer(test)
	composer.body = hostile + strings.Repeat(" long", 60)
	composer.press("c")
	configuration, _ := composer.lastConfirm()
	if strings.ContainsFunc(configuration.Text, func(character rune) bool { return character < 0x20 || (character >= 0x7f && character < 0xa0) }) {
		test.Errorf("control characters in %q", configuration.Text)
	}
	if width := ansi.StringWidth(configuration.Text); width > composer.view.width {
		test.Errorf("text is %d wide, view is %d", width, composer.view.width)
	}
}

func TestReplyAndCommentAreInHelp(test *testing.T) {
	harness := loaded(test, 100, 30)
	got := map[string]string{}
	for _, bindings := range harness.view.FullHelp() {
		for _, binding := range bindings {
			if k := binding.Keys(); len(k) == 1 && (k[0] == "r" || k[0] == "c") {
				got[k[0]] = binding.Help().Desc
			}
		}
	}
	if len(got) != 2 || got["r"] == "" || got["c"] == "" {
		test.Errorf("help = %v", got)
	}
}
