package workspace

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

const (
	headSHA                   = "aaaa1111"
	pullRequestNodeIdentifier = "PR_node_7"
)

// reviewer is a composer whose pull request has a head commit.
func newReviewer(test *testing.T) *composer {
	composer := newComposer(test)
	detail := testDetail()
	detail.Identifier = pullRequestNodeIdentifier
	composer.client.SetPullRequest(detail)
	composer.setHead(headSHA)
	return composer
}

// setHead moves the head on the server and, when load is true, refreshes the view.
func (composer *composer) setHead(sha string) {
	composer.client.SetHeadSHA(testRepository, 7, sha)
	composer.press("R")
}

func (composer *composer) command(name string) {
	composer.test.Helper()
	command, ok := composer.view.Command(name, "")
	if !ok {
		composer.test.Fatalf("command %q not handled", name)
	}
	composer.run(command)
}

func (composer *composer) lastError() string {
	if err := composer.lastErr(); err != nil {
		return err.Error()
	}
	return ""
}

func TestReviewVerdicts(test *testing.T) {
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
			want:     fake.Submission{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Event: github.ReviewEventApprove, CommitObjectIdentifier: headSHA},
			info:     "Approved #7",
		},
		{
			name: "approve with a summary", command: "approve", body: "Nice.\n",
			wantText: "Approve #7 in acme/widgets?",
			want:     fake.Submission{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Event: github.ReviewEventApprove, Body: "Nice.", CommitObjectIdentifier: headSHA},
			info:     "Approved #7",
		},
		{
			name: "request changes", command: "request-changes", body: "Please fix.",
			wantText: "Request changes on #7 in acme/widgets?",
			want:     fake.Submission{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Event: github.ReviewEventRequestChanges, Body: "Please fix.", CommitObjectIdentifier: headSHA},
			info:     "Requested changes on #7",
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			composer := newReviewer(test)
			composer.body = testCase.body
			composer.command(testCase.command)
			configuration, ok := composer.lastConfirm()
			if !ok {
				test.Fatalf("no confirmation; error: %q", composer.lastError())
			}
			if configuration.Text != testCase.wantText || configuration.Notice {
				test.Errorf("confirm = %+v, want text %q", configuration, testCase.wantText)
			}
			if len(composer.client.Submissions()) != 0 {
				test.Fatal("submitted before the answer")
			}
			composer.run(configuration.OnYes)
			if diff := cmp.Diff([]fake.Submission{testCase.want}, composer.client.Submissions()); diff != "" {
				test.Errorf("submissions (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"Submitting review…", testCase.info}, composer.infoMessages()); diff != "" {
				test.Errorf("info (-want +got):\n%s", diff)
			}
			if composer.view.review.draft != "" {
				test.Errorf("draft kept after success: %q", composer.view.review.draft)
			}
			if reviews := composer.view.detail.Reviews; reviews[len(reviews)-1].Body != testCase.want.Body || reviews[len(reviews)-1].SubmittedAt.IsZero() {
				test.Errorf("workspace not refreshed after success: %+v", reviews[len(reviews)-1])
			}
		})
	}
}

func TestReviewCommentThroughChooser(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "Some thoughts."
	composer.command("review")
	if !composer.view.CapturesKeys() {
		test.Fatal("chooser does not capture keys")
	}
	composer.typeText("com")
	composer.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if composer.view.CapturesKeys() {
		test.Error("chooser still open after choosing")
	}
	configuration, ok := composer.lastConfirm()
	if !ok || configuration.Text != "Comment on #7 in acme/widgets?" {
		test.Fatalf("confirm = %+v, %v", configuration, ok)
	}
	composer.run(configuration.OnYes)
	submissions := composer.client.Submissions()
	if len(submissions) != 1 || submissions[0].Event != github.ReviewEventComment {
		test.Errorf("submissions = %+v", submissions)
	}
}

func TestReviewCommandReview(test *testing.T) {
	composer := newReviewer(test)
	composer.command("review")
	if composer.view.review.chooser == nil {
		test.Fatal(":review did not open the chooser")
	}
	if !strings.Contains(composer.render(), "Request changes") {
		test.Error("chooser not drawn")
	}
}

func TestReviewChooserCancels(test *testing.T) {
	composer := newReviewer(test)
	composer.command("review")
	composer.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if composer.view.CapturesKeys() || composer.view.review.chooser != nil {
		test.Error("esc did not close the chooser")
	}
	if len(composer.openedOn) != 0 || len(composer.client.Submissions()) != 0 {
		test.Error("cancelling did something")
	}
}

// TestReviewChooserReleasesOnEsc drives the chooser through the real root, so
// key routing is covered too.
func TestReviewChooserReleasesOnEsc(test *testing.T) {
	harness := loaded(test, 100, 24)
	model := userinterface.New(harness.view, theme.New(true), userinterface.Options{})
	step := func(message tea.Msg) { _, _ = model.Update(message) }
	step(tea.WindowSizeMsg{Width: 100, Height: 25})
	step(tea.KeyPressMsg{Code: ':', Text: ":"})
	for _, character := range "review" {
		step(tea.KeyPressMsg{Code: character, Text: string(character)})
	}
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command != nil {
		step(command())
	}
	if !harness.view.CapturesKeys() {
		test.Fatal(":review through the root should open the chooser")
	}
	step(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if harness.view.review.chooser.Query() != "j" {
		test.Errorf("key did not reach the chooser raw: %q", harness.view.review.chooser.Query())
	}
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if harness.view.CapturesKeys() {
		test.Error("view still captures keys after esc")
	}
}

func TestReviewEmptyBodyIsAnError(test *testing.T) {
	for _, name := range []string{"request-changes", "comment"} {
		test.Run(name, func(test *testing.T) {
			composer := newReviewer(test)
			composer.body = "  \n"
			if name == "comment" {
				composer.view.review.chooser = nil
				command := composer.view.review.start(composer.view.reviewEnvironment(), github.ReviewEventComment)
				composer.run(command)
			} else {
				composer.command(name)
			}
			if _, ok := composer.lastConfirm(); ok {
				test.Error("asked for confirmation")
			}
			if !strings.Contains(composer.lastError(), "empty review") {
				test.Errorf("error = %q", composer.lastError())
			}
			if len(composer.client.Submissions()) != 0 {
				test.Error("something was sent")
			}
		})
	}
}

func TestReviewWithPendingReview(test *testing.T) {
	composer := newReviewer(test)
	composer.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "PRR_1", Body: "draft on github", CommentCount: 3})
	composer.body = "" // an empty body is fine with inline comments
	command := composer.view.review.start(composer.view.reviewEnvironment(), github.ReviewEventComment)
	composer.run(command)
	if diff := cmp.Diff([]string{"draft on github"}, composer.openedOn); diff != "" {
		test.Errorf("editor opened on (-want +got):\n%s", diff)
	}
	configuration, ok := composer.lastConfirm()
	if !ok {
		test.Fatalf("no confirmation; error: %q", composer.lastError())
	}
	if want := "Comment on #7 in acme/widgets with 3 pending comments?"; configuration.Text != want {
		test.Errorf("text = %q, want %q", configuration.Text, want)
	}
	composer.run(configuration.OnYes)
	want := []fake.Submission{{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Event: github.ReviewEventComment, CommitObjectIdentifier: headSHA, PendingReviewIdentifier: "PRR_1"}}
	if diff := cmp.Diff(want, composer.client.Submissions()); diff != "" {
		test.Errorf("submissions (-want +got):\n%s", diff)
	}
}

func TestReviewPendingGoneAtSubmit(test *testing.T) {
	composer := newReviewer(test)
	composer.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "PRR_1", CommentCount: 1})
	composer.body = "my summary"
	composer.command("approve")
	configuration, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no confirmation")
	}
	composer.client.SetPendingReview(testRepository, 7, nil) // submitted elsewhere meanwhile
	before := len(composer.client.Calls())
	composer.run(configuration.OnYes)
	if len(composer.client.Submissions()) != 0 {
		test.Error("submitted")
	}
	if got := composer.lastError(); !strings.Contains(got, "submitted or deleted elsewhere") {
		test.Errorf("error = %q", got)
	}
	if composer.view.review.draft != "my summary" {
		test.Errorf("draft = %q", composer.view.review.draft)
	}
	var reloaded bool
	for _, call := range composer.client.Calls()[before:] {
		reloaded = reloaded || call.Method == fake.MethodGetPullRequest
	}
	if !reloaded {
		test.Error("workspace did not refresh")
	}
}

func TestReviewHeadMoved(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "ok"
	composer.client.SetHeadSHA(testRepository, 7, "bbbb2222") // pushed after the view loaded
	composer.command("approve")
	configuration, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no notice shown")
	}
	if !configuration.Notice || !strings.Contains(configuration.Text, "Head moved") || configuration.OnYes != nil {
		test.Errorf("confirm = %+v", configuration)
	}
	composer.run(configuration.OnDismiss) // any key
	if len(composer.client.Submissions()) != 0 {
		test.Error("submitted although the head moved")
	}
	if composer.view.detail.HeadSHA != "bbbb2222" {
		test.Errorf("workspace head = %q, want it refreshed", composer.view.detail.HeadSHA)
	}
}

func TestReviewServerErrorKeepsDraft(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "needs work"
	composer.command("request-changes")
	configuration, _ := composer.lastConfirm()
	composer.client.SetError(fake.MethodSubmitReview, errors.New("Can not request changes on your own pull request\x1b[31m"))
	composer.run(configuration.OnYes)
	got := composer.lastError()
	if !strings.Contains(got, "Can not request changes on your own pull request") || !strings.Contains(got, "refreshed to show the current state") || strings.ContainsRune(got, 0x1b) {
		test.Errorf("error = %q", got)
	}
	if len(composer.infoMessages()) != 1 {
		test.Errorf("success message after failure: %v", composer.infoMessages())
	}
	// The next attempt starts from the draft.
	composer.client.SetError(fake.MethodSubmitReview, nil)
	composer.openedOn = nil
	composer.command("request-changes")
	if diff := cmp.Diff([]string{"needs work"}, composer.openedOn); diff != "" {
		test.Errorf("editor opened on (-want +got):\n%s", diff)
	}
}

func TestReviewDeclinedKeepsDraft(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "maybe"
	composer.command("approve")
	if _, ok := composer.lastConfirm(); !ok {
		test.Fatal("no confirmation")
	}
	// Declining is the shell dropping OnYes.
	composer.openedOn = nil
	composer.command("approve")
	if diff := cmp.Diff([]string{"maybe"}, composer.openedOn); diff != "" {
		test.Errorf("editor opened on (-want +got):\n%s", diff)
	}
}

func TestReviewEditorFailureKeepsText(test *testing.T) {
	composer := newReviewer(test)
	composer.body, composer.err = "typed before crash", errors.New("exit status 1")
	composer.command("approve")
	if _, ok := composer.lastConfirm(); ok {
		test.Error("asked for confirmation after editor failure")
	}
	if !strings.Contains(composer.lastError(), "edit review") || composer.view.review.draft != "typed before crash" {
		test.Errorf("error %q, draft %q", composer.lastError(), composer.view.review.draft)
	}
}

func TestReviewStatusErrorIsShown(test *testing.T) {
	composer := newReviewer(test)
	composer.client.SetError(fake.MethodGetReviewStatus, errors.New("boom\x1b[31m"))
	composer.command("approve")
	if got := composer.lastError(); !strings.Contains(got, "boom") || strings.ContainsRune(got, 0x1b) {
		test.Errorf("error = %q", got)
	}
	if len(composer.openedOn) != 0 {
		test.Error("editor opened")
	}
}

func TestReviewStaleResultsAreIgnored(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "x"
	old := composer.view.review.start(composer.view.reviewEnvironment(), github.ReviewEventApprove)
	composer.view.review.sequence++ // another review was started meanwhile
	composer.run(old)
	if len(composer.openedOn) != 0 {
		test.Error("stale status opened the editor")
	}
	// A result for another workspace is ignored too.
	composer.send(reviewSubmittedMessage{flow: nil, repository: testRepository, number: 7, event: github.ReviewEventApprove})
	if len(composer.infoMessages()) != 0 {
		test.Errorf("foreign result acted on: %v", composer.infoMessages())
	}
}

func TestCommandsReachOnlyKnownNames(test *testing.T) {
	view := newHarness(test, 80, 24).view
	for _, name := range []string{"approve", "request-changes", "review"} {
		if _, ok := view.Command(name, ""); !ok {
			test.Errorf("%s not handled", name)
		}
	}
	if command, ok := view.Command("frobnicate", ""); ok || command != nil {
		test.Error("unknown command handled")
	}
}

func TestReviewBeforeLoad(test *testing.T) {
	view := newHarness(test, 80, 24).view // Init not run: nothing loaded
	messageList := messages(view.review.openChooser(view.reviewEnvironment()))
	if len(messageList) != 1 {
		test.Fatalf("msgs = %v", messageList)
	}
	if i, ok := messageList[0].(userinterface.InfoMessage); !ok || i.Text != "pull request not loaded yet" {
		test.Errorf("msg = %#v", messageList[0])
	}
}

func TestReviewQuestion(test *testing.T) {
	pending := func(count int) *github.PendingReview { return &github.PendingReview{CommentCount: count} }
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
	for _, testCase := range tests {
		got := reviewQuestion(testCase.event, github.Repository{Owner: "o", Name: "r"}, 42, testCase.pending)
		if got != testCase.want {
			test.Errorf("%s: got %q, want %q", testCase.event, got, testCase.want)
		}
	}
}
