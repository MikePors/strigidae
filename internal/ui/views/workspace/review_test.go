package workspace

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

const (
	headSHA  = "aaaa1111"
	prNodeID = "PR_node_7"
)

// reviewer is a composer whose pull request has a head commit.
func newReviewer(t *testing.T) *composer {
	c := newComposer(t)
	d := testDetail()
	d.ID = prNodeID
	c.client.SetPullRequest(d)
	c.setHead(headSHA)
	return c
}

// setHead moves the head on the server and, when load is true, refreshes the view.
func (c *composer) setHead(sha string) {
	c.client.SetHeadSHA(testRepo, 7, sha)
	c.press("R")
}

func (c *composer) command(name string) {
	c.t.Helper()
	cmd, ok := c.v.Command(name, "")
	if !ok {
		c.t.Fatalf("command %q not handled", name)
	}
	c.run(cmd)
}

func (c *composer) lastError() string {
	if err := c.lastErr(); err != nil {
		return err.Error()
	}
	return ""
}

func TestReviewVerdicts(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		body     string
		wantText string
		want     fake.Submission
		info     string
	}{
		{
			name: "approve with no comment", command: "approve", body: "",
			wantText: "Approve #7 in acme/widgets?",
			want:     fake.Submission{Repo: testRepo, Number: 7, PullRequestID: prNodeID, Event: github.ReviewEventApprove, CommitOID: headSHA},
			info:     "Approved #7",
		},
		{
			name: "approve with a summary", command: "approve", body: "Nice.\n",
			wantText: "Approve #7 in acme/widgets?",
			want:     fake.Submission{Repo: testRepo, Number: 7, PullRequestID: prNodeID, Event: github.ReviewEventApprove, Body: "Nice.", CommitOID: headSHA},
			info:     "Approved #7",
		},
		{
			name: "request changes", command: "request-changes", body: "Please fix.",
			wantText: "Request changes on #7 in acme/widgets?",
			want:     fake.Submission{Repo: testRepo, Number: 7, PullRequestID: prNodeID, Event: github.ReviewEventRequestChanges, Body: "Please fix.", CommitOID: headSHA},
			info:     "Requested changes on #7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newReviewer(t)
			c.body = tt.body
			c.command(tt.command)
			conf, ok := c.lastConfirm()
			if !ok {
				t.Fatalf("no confirmation; error: %q", c.lastError())
			}
			if conf.Text != tt.wantText || conf.Notice {
				t.Errorf("confirm = %+v, want text %q", conf, tt.wantText)
			}
			if len(c.client.Submissions()) != 0 {
				t.Fatal("submitted before the answer")
			}
			c.run(conf.OnYes)
			if diff := cmp.Diff([]fake.Submission{tt.want}, c.client.Submissions()); diff != "" {
				t.Errorf("submissions (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"Submitting review…", tt.info}, c.infos()); diff != "" {
				t.Errorf("info (-want +got):\n%s", diff)
			}
			if c.v.reviewDraft != "" {
				t.Errorf("draft kept after success: %q", c.v.reviewDraft)
			}
			if rs := c.v.detail.Reviews; rs[len(rs)-1].Body != tt.want.Body || rs[len(rs)-1].SubmittedAt.IsZero() {
				t.Errorf("workspace not refreshed after success: %+v", rs[len(rs)-1])
			}
		})
	}
}

func TestReviewCommentThroughChooser(t *testing.T) {
	c := newReviewer(t)
	c.body = "Some thoughts."
	c.command("review")
	if !c.v.CapturesKeys() {
		t.Fatal("chooser does not capture keys")
	}
	c.typ("com")
	c.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if c.v.CapturesKeys() {
		t.Error("chooser still open after choosing")
	}
	conf, ok := c.lastConfirm()
	if !ok || conf.Text != "Comment on #7 in acme/widgets?" {
		t.Fatalf("confirm = %+v, %v", conf, ok)
	}
	c.run(conf.OnYes)
	subs := c.client.Submissions()
	if len(subs) != 1 || subs[0].Event != github.ReviewEventComment {
		t.Errorf("submissions = %+v", subs)
	}
}

func TestReviewCommandReview(t *testing.T) {
	c := newReviewer(t)
	c.command("review")
	if c.v.chooser == nil {
		t.Fatal(":review did not open the chooser")
	}
	if !strings.Contains(c.render(), "Request changes") {
		t.Error("chooser not drawn")
	}
}

func TestReviewChooserCancels(t *testing.T) {
	c := newReviewer(t)
	c.command("review")
	c.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if c.v.CapturesKeys() || c.v.chooser != nil {
		t.Error("esc did not close the chooser")
	}
	if len(c.openedOn) != 0 || len(c.client.Submissions()) != 0 {
		t.Error("cancelling did something")
	}
}

// TestReviewChooserReleasesOnEsc drives the chooser through the real root, so
// key routing is covered too.
func TestReviewChooserReleasesOnEsc(t *testing.T) {
	h := loaded(t, 100, 24)
	m := ui.New(h.v, theme.New(true), ui.Options{})
	step := func(msg tea.Msg) { _, _ = m.Update(msg) }
	step(tea.WindowSizeMsg{Width: 100, Height: 25})
	step(tea.KeyPressMsg{Code: ':', Text: ":"})
	for _, r := range "review" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		step(cmd())
	}
	if !h.v.CapturesKeys() {
		t.Fatal(":review through the root should open the chooser")
	}
	step(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if h.v.chooser.Query() != "j" {
		t.Errorf("key did not reach the chooser raw: %q", h.v.chooser.Query())
	}
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.v.CapturesKeys() {
		t.Error("view still captures keys after esc")
	}
}

func TestReviewEmptyBodyIsAnError(t *testing.T) {
	for _, name := range []string{"request-changes", "comment"} {
		t.Run(name, func(t *testing.T) {
			c := newReviewer(t)
			c.body = "  \n"
			if name == "comment" {
				c.v.chooser = nil
				cmd := c.v.startReview(github.ReviewEventComment)
				c.run(cmd)
			} else {
				c.command(name)
			}
			if _, ok := c.lastConfirm(); ok {
				t.Error("asked for confirmation")
			}
			if !strings.Contains(c.lastError(), "empty review") {
				t.Errorf("error = %q", c.lastError())
			}
			if len(c.client.Submissions()) != 0 {
				t.Error("something was sent")
			}
		})
	}
}

func TestReviewWithPendingReview(t *testing.T) {
	c := newReviewer(t)
	c.client.SetPendingReview(testRepo, 7, &github.PendingReview{ID: "PRR_1", Body: "draft on github", CommentCount: 3})
	c.body = "" // an empty body is fine with inline comments
	cmd := c.v.startReview(github.ReviewEventComment)
	c.run(cmd)
	if diff := cmp.Diff([]string{"draft on github"}, c.openedOn); diff != "" {
		t.Errorf("editor opened on (-want +got):\n%s", diff)
	}
	conf, ok := c.lastConfirm()
	if !ok {
		t.Fatalf("no confirmation; error: %q", c.lastError())
	}
	if want := "Comment on #7 in acme/widgets with 3 pending comments?"; conf.Text != want {
		t.Errorf("text = %q, want %q", conf.Text, want)
	}
	c.run(conf.OnYes)
	want := []fake.Submission{{Repo: testRepo, Number: 7, PullRequestID: prNodeID, Event: github.ReviewEventComment, CommitOID: headSHA, PendingReviewID: "PRR_1"}}
	if diff := cmp.Diff(want, c.client.Submissions()); diff != "" {
		t.Errorf("submissions (-want +got):\n%s", diff)
	}
}

func TestReviewPendingGoneAtSubmit(t *testing.T) {
	c := newReviewer(t)
	c.client.SetPendingReview(testRepo, 7, &github.PendingReview{ID: "PRR_1", CommentCount: 1})
	c.body = "my summary"
	c.command("approve")
	conf, ok := c.lastConfirm()
	if !ok {
		t.Fatal("no confirmation")
	}
	c.client.SetPendingReview(testRepo, 7, nil) // submitted elsewhere meanwhile
	before := len(c.client.Calls())
	c.run(conf.OnYes)
	if len(c.client.Submissions()) != 0 {
		t.Error("submitted")
	}
	if got := c.lastError(); !strings.Contains(got, "submitted or deleted elsewhere") {
		t.Errorf("error = %q", got)
	}
	if c.v.reviewDraft != "my summary" {
		t.Errorf("draft = %q", c.v.reviewDraft)
	}
	var reloaded bool
	for _, call := range c.client.Calls()[before:] {
		reloaded = reloaded || call.Method == fake.MethodGetPullRequest
	}
	if !reloaded {
		t.Error("workspace did not refresh")
	}
}

func TestReviewHeadMoved(t *testing.T) {
	c := newReviewer(t)
	c.body = "ok"
	c.client.SetHeadSHA(testRepo, 7, "bbbb2222") // pushed after the view loaded
	c.command("approve")
	conf, ok := c.lastConfirm()
	if !ok {
		t.Fatal("no notice shown")
	}
	if !conf.Notice || !strings.Contains(conf.Text, "Head moved") || conf.OnYes != nil {
		t.Errorf("confirm = %+v", conf)
	}
	c.run(conf.OnDismiss) // any key
	if len(c.client.Submissions()) != 0 {
		t.Error("submitted although the head moved")
	}
	if c.v.detail.HeadSHA != "bbbb2222" {
		t.Errorf("workspace head = %q, want it refreshed", c.v.detail.HeadSHA)
	}
}

func TestReviewServerErrorKeepsDraft(t *testing.T) {
	c := newReviewer(t)
	c.body = "needs work"
	c.command("request-changes")
	conf, _ := c.lastConfirm()
	c.client.SetError(fake.MethodSubmitReview, errors.New("Can not request changes on your own pull request\x1b[31m"))
	c.run(conf.OnYes)
	got := c.lastError()
	if !strings.Contains(got, "Can not request changes on your own pull request") || !strings.Contains(got, "refreshed to show the current state") || strings.ContainsRune(got, 0x1b) {
		t.Errorf("error = %q", got)
	}
	if len(c.infos()) != 1 {
		t.Errorf("success message after failure: %v", c.infos())
	}
	// The next attempt starts from the draft.
	c.client.SetError(fake.MethodSubmitReview, nil)
	c.openedOn = nil
	c.command("request-changes")
	if diff := cmp.Diff([]string{"needs work"}, c.openedOn); diff != "" {
		t.Errorf("editor opened on (-want +got):\n%s", diff)
	}
}

func TestReviewDeclinedKeepsDraft(t *testing.T) {
	c := newReviewer(t)
	c.body = "maybe"
	c.command("approve")
	if _, ok := c.lastConfirm(); !ok {
		t.Fatal("no confirmation")
	}
	// Declining is the shell dropping OnYes.
	c.openedOn = nil
	c.command("approve")
	if diff := cmp.Diff([]string{"maybe"}, c.openedOn); diff != "" {
		t.Errorf("editor opened on (-want +got):\n%s", diff)
	}
}

func TestReviewEditorFailureKeepsText(t *testing.T) {
	c := newReviewer(t)
	c.body, c.err = "typed before crash", errors.New("exit status 1")
	c.command("approve")
	if _, ok := c.lastConfirm(); ok {
		t.Error("asked for confirmation after editor failure")
	}
	if !strings.Contains(c.lastError(), "edit review") || c.v.reviewDraft != "typed before crash" {
		t.Errorf("error %q, draft %q", c.lastError(), c.v.reviewDraft)
	}
}

func TestReviewStatusErrorIsShown(t *testing.T) {
	c := newReviewer(t)
	c.client.SetError(fake.MethodGetReviewStatus, errors.New("boom\x1b[31m"))
	c.command("approve")
	if got := c.lastError(); !strings.Contains(got, "boom") || strings.ContainsRune(got, 0x1b) {
		t.Errorf("error = %q", got)
	}
	if len(c.openedOn) != 0 {
		t.Error("editor opened")
	}
}

func TestReviewStaleResultsAreIgnored(t *testing.T) {
	c := newReviewer(t)
	c.body = "x"
	old := c.v.startReview(github.ReviewEventApprove)
	c.v.rseq++ // another review was started meanwhile
	c.run(old)
	if len(c.openedOn) != 0 {
		t.Error("stale status opened the editor")
	}
	// A result for another workspace is ignored too.
	c.send(reviewSubmittedMsg{owner: nil, repo: testRepo, number: 7, event: github.ReviewEventApprove})
	if len(c.infos()) != 0 {
		t.Errorf("foreign result acted on: %v", c.infos())
	}
}

func TestCommandsReachOnlyKnownNames(t *testing.T) {
	v := newHarness(t, 80, 24).v
	for _, name := range []string{"approve", "request-changes", "review"} {
		if _, ok := v.Command(name, ""); !ok {
			t.Errorf("%s not handled", name)
		}
	}
	if cmd, ok := v.Command("frobnicate", ""); ok || cmd != nil {
		t.Error("unknown command handled")
	}
}

func TestReviewBeforeLoad(t *testing.T) {
	v := newHarness(t, 80, 24).v // Init not run: nothing loaded
	msgs := messages(v.openChooser())
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v", msgs)
	}
	if i, ok := msgs[0].(ui.InfoMsg); !ok || i.Text != "pull request not loaded yet" {
		t.Errorf("msg = %#v", msgs[0])
	}
}

func TestReviewQuestion(t *testing.T) {
	pending := func(n int) *github.PendingReview { return &github.PendingReview{CommentCount: n} }
	tests := []struct {
		event   github.ReviewEvent
		pending *github.PendingReview
		want    string
	}{
		{github.ReviewEventApprove, nil, "Approve #42 in o/r?"},
		{github.ReviewEventApprove, pending(0), "Approve #42 in o/r?"},
		{github.ReviewEventApprove, pending(1), "Approve #42 in o/r with 1 pending comment?"},
		{github.ReviewEventRequestChanges, pending(3), "Request changes on #42 in o/r with 3 pending comments?"},
		{github.ReviewEventComment, nil, "Comment on #42 in o/r?"},
	}
	for _, tt := range tests {
		got := reviewQuestion(tt.event, github.Repo{Owner: "o", Name: "r"}, 42, tt.pending)
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.event, got, tt.want)
		}
	}
}
