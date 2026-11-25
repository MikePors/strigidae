package workspace

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func TestParseReviewArguments(test *testing.T) {
	tests := []struct {
		name         string
		arguments    string
		verdictFirst bool
		want         reviewArguments
		wantError    string
	}{
		{name: "none", arguments: "", want: reviewArguments{}},
		{name: "message", arguments: "-m LGTM, thanks", want: reviewArguments{message: "LGTM, thanks", hasMessage: true}},
		{name: "double quotes", arguments: `-m "Looks good"`, want: reviewArguments{message: "Looks good", hasMessage: true}},
		{name: "single quotes", arguments: `-m 'Looks good'`, want: reviewArguments{message: "Looks good", hasMessage: true}},
		{name: "empty message", arguments: `-m ""`, want: reviewArguments{hasMessage: true}},
		{name: "one quote kept", arguments: `-m "unfinished`, want: reviewArguments{message: `"unfinished`, hasMessage: true}},
		{name: "no message", arguments: "-m", wantError: "-m needs a message"},
		{name: "unknown argument", arguments: "--empty", wantError: `unknown argument "--empty"`},
		{name: "verdict word without its own command", arguments: "approve", wantError: `unknown argument "approve"`},
		{name: "review chooser", arguments: "", verdictFirst: true, want: reviewArguments{}},
		{name: "review verdict", arguments: "comment", verdictFirst: true, want: reviewArguments{event: github.ReviewEventComment}},
		{name: "review verdict and message", arguments: "request-changes -m Please fix", verdictFirst: true, want: reviewArguments{event: github.ReviewEventRequestChanges, message: "Please fix", hasMessage: true}},
		{name: "review message without verdict", arguments: "-m hi", verdictFirst: true, wantError: "-m needs a verdict first"},
		{name: "review unknown verdict", arguments: "ship", verdictFirst: true, wantError: `unknown verdict "ship"`},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got, err := parseReviewArguments(testCase.arguments, testCase.verdictFirst)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					test.Fatalf("error = %v, want one containing %q", err, testCase.wantError)
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got, cmp.AllowUnexported(reviewArguments{})); diff != "" {
				test.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// commandWith runs a command with arguments.
func (composer *composer) commandWith(name, arguments string) {
	composer.test.Helper()
	command, ok := composer.view.Command(name, arguments)
	if !ok {
		composer.test.Fatalf("command %q not handled", name)
	}
	composer.run(command)
}

func TestReviewMessageSkipsTheEditor(test *testing.T) {
	tests := []struct {
		name      string
		command   string
		arguments string
		wantEvent github.ReviewEvent
		wantBody  string
		wantText  string
	}{
		{"approve", "approve", `-m "Nice work"`, github.ReviewEventApprove, "Nice work", "Approve #7 in acme/widgets?"},
		{"approve without a summary", "approve", `-m ""`, github.ReviewEventApprove, "", "Approve #7 in acme/widgets?"},
		{"request changes", "request-changes", "-m Please fix", github.ReviewEventRequestChanges, "Please fix", "Request changes on #7 in acme/widgets?"},
		{"review verdict", "review", "comment -m Some thoughts", github.ReviewEventComment, "Some thoughts", "Comment on #7 in acme/widgets?"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			composer := newReviewer(test)
			composer.body = "text from the editor"
			composer.commandWith(testCase.command, testCase.arguments)
			confirmation, ok := composer.lastConfirm()
			if !ok || confirmation.Text != testCase.wantText {
				test.Fatalf("confirm = %+v, %v; error %q", confirmation, ok, composer.lastError())
			}
			if len(composer.client.Submissions()) != 0 {
				test.Fatal("submitted before the answer")
			}
			composer.run(confirmation.OnYes)
			submissions := composer.client.Submissions()
			if len(submissions) != 1 || submissions[0].Event != testCase.wantEvent || submissions[0].Body != testCase.wantBody {
				test.Errorf("submissions = %+v", submissions)
			}
		})
	}
}

func TestReviewVerdictWordSkipsTheChooser(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "from the editor"
	composer.commandWith("review", "request-changes")
	if composer.view.review.chooser != nil {
		test.Error("the chooser opened")
	}
	confirmation, ok := composer.lastConfirm()
	if !ok || confirmation.Text != "Request changes on #7 in acme/widgets?" {
		test.Fatalf("confirm = %+v, %v", confirmation, ok)
	}
	composer.run(confirmation.OnYes)
	submissions := composer.client.Submissions()
	if len(submissions) != 1 || submissions[0].Body != "from the editor" {
		test.Errorf("submissions = %+v", submissions)
	}
}

func TestReviewEmptyMessageStillRefusedWhereEmptyIsNotAllowed(test *testing.T) {
	composer := newReviewer(test)
	composer.commandWith("request-changes", `-m ""`)
	if _, ok := composer.lastConfirm(); ok {
		test.Error("asked for confirmation")
	}
	if !strings.Contains(composer.lastError(), "empty review") {
		test.Errorf("error = %q", composer.lastError())
	}
	if len(composer.client.Submissions()) != 0 {
		test.Error("something was sent")
	}
}

func TestReviewBadArgumentsSendNothing(test *testing.T) {
	for _, testCase := range []struct{ command, arguments, want string }{
		{"approve", "-m", "-m needs a message"},
		{"approve", "--empty", "unknown argument"},
		{"request-changes", "oops", "unknown argument"},
		{"review", "-m hi", "needs a verdict"},
		{"review", "ship", "unknown verdict"},
	} {
		test.Run(testCase.command+" "+testCase.arguments, func(test *testing.T) {
			composer := newReviewer(test)
			composer.commandWith(testCase.command, testCase.arguments)
			if !strings.Contains(composer.lastError(), testCase.want) {
				test.Errorf("error = %q, want %q", composer.lastError(), testCase.want)
			}
			if _, ok := composer.lastConfirm(); ok || composer.view.review.chooser != nil || len(composer.client.Submissions()) != 0 {
				test.Error("something started")
			}
		})
	}
}

func TestReviewMessageOnOwnPullRequestIsRefused(test *testing.T) {
	composer := newReviewer(test)
	composer.view.detail.ViewerDidAuthor = true
	before := composer.callCount(fake.MethodGetReviewStatus)
	composer.commandWith("approve", "-m hi")
	if _, ok := composer.lastConfirm(); ok {
		test.Error("asked for confirmation")
	}
	if diff := cmp.Diff([]string{ownPullRequestText}, composer.infoMessages()); diff != "" {
		test.Errorf("notice (-want +got):\n%s", diff)
	}
	if count := composer.callCount(fake.MethodGetReviewStatus) - before; count != 0 {
		test.Errorf("asked GitHub for the review state %d times", count)
	}
}

func TestReviewMessageKeepsTheEditorDraft(test *testing.T) {
	composer := newReviewer(test)
	composer.view.review.draft = "long summary from the editor"
	composer.body = "unused: the editor must not open"
	composer.commandWith("approve", "-m quick")
	if _, ok := composer.lastConfirm(); !ok {
		test.Fatalf("no confirmation; error %q", composer.lastError())
	}
	if composer.view.review.draft != "long summary from the editor" {
		test.Errorf("draft = %q, replaced by the command line", composer.view.review.draft)
	}
	composer.commandWith("request-changes", "-m also quick")
	if composer.view.review.draft != "long summary from the editor" {
		test.Errorf("draft = %q after a second attempt", composer.view.review.draft)
	}
}

func TestReviewEmptyMessageDoesNotBlameTheEditor(test *testing.T) {
	composer := newReviewer(test)
	composer.commandWith("request-changes", `-m ""`)
	if strings.Contains(composer.lastError(), "editor") {
		test.Errorf("error = %q", composer.lastError())
	}
}

func TestReviewBadArgumentsAreReportedBeforeLoading(test *testing.T) {
	view := newHarness(test, 80, 24).view // Init not run: nothing loaded
	command, ok := view.Command("approve", "--empty")
	if !ok {
		test.Fatal("command not handled")
	}
	messageList := messages(command)
	if len(messageList) != 1 {
		test.Fatalf("messages = %v", messageList)
	}
	failure, isError := messageList[0].(userinterface.ErrorMessage)
	if !isError || !strings.Contains(failure.Err.Error(), "unknown argument") {
		test.Errorf("message = %#v", messageList[0])
	}
}
