package workspace

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// editPullRequest runs :edit-pull-request with the editor returning text.
func (inliner *inliner) editPullRequest(text string) {
	inliner.test.Helper()
	inliner.body = text
	inliner.messages, inliner.openedOn = nil, nil
	inliner.command("edit-pull-request")
}

func (inliner *inliner) pullRequestUpdates() []github.UpdatePullRequestInput {
	return inliner.client.PullRequestUpdates()
}

func TestEditPullRequestStartsFromTheCurrentText(test *testing.T) {
	inliner := newInliner(test)
	// Changed on GitHub after the workspace loaded.
	detail := testDetail()
	detail.Identifier, detail.ViewerCanUpdate = pullRequestNodeIdentifier, true
	detail.Title, detail.Body = "Newer title", "Newer\r\n\r\nbody"
	inliner.client.SetPullRequest(detail)
	inliner.editPullRequest("Newer title\n\nNewer\n\nbody")
	if diff := cmp.Diff([]string{"Newer title\n\nNewer\r\n\r\nbody"}, inliner.openedOn); diff != "" {
		test.Errorf("editor text (-want +got):\n%s", diff)
	}
	if got := inliner.infoMessages(); len(got) != 1 || got[0] != noticeNoChanges {
		test.Errorf("notices = %v, want no changes (CRLF is not a change)", got)
	}
	if len(inliner.pullRequestUpdates()) != 0 {
		test.Error("something was sent")
	}
}

func TestEditPullRequestAsksThenSendsBothFields(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("New title\n\nFirst\n\n\nSecond  \n\n")
	confirmation, asked := inliner.lastConfirm()
	if !asked {
		test.Fatalf("no question; messages %v", inliner.messages)
	}
	if want := "Update the title and body of #7?"; confirmation.Text != want {
		test.Errorf("question = %q, want %q", confirmation.Text, want)
	}
	if len(inliner.pullRequestUpdates()) != 0 || inliner.view.writes != 0 {
		test.Fatal("written or busy before the answer")
	}
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	want := []github.UpdatePullRequestInput{{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Title: "New title", Body: "First\n\n\nSecond"}}
	if diff := cmp.Diff(want, inliner.pullRequestUpdates()); diff != "" {
		test.Errorf("update (-want +got):\n%s", diff)
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight: %d", inliner.view.writes)
	}
	if got := inliner.infoMessages(); !contains(got, noticePullRequestUpdated) {
		test.Errorf("notices = %v", got)
	}
	if inliner.view.detail.Title != "New title" || inliner.view.detail.Body != "First\n\n\nSecond" {
		test.Errorf("detail = %q, %q", inliner.view.detail.Title, inliner.view.detail.Body)
	}
	inliner.press("h", "g g") // the Overview
	if shown := inliner.shown(); !strings.Contains(shown, "New title") || !strings.Contains(shown, "Second") {
		test.Errorf("the screen does not show the new text:\n%s", shown)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestEditPullRequestRefusals(test *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantError string
		wantInfo  string
	}{
		{"empty title", "\n\nBody", "empty title", ""},
		{"no blank line", "Title\nBody", "blank line", ""},
		{"unchanged", editorTextOfTestDetail(), "", noticeNoChanges},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.editPullRequest(testCase.text)
			if _, asked := inliner.lastConfirm(); asked {
				test.Error("a question was asked")
			}
			err := inliner.lastErr()
			switch {
			case testCase.wantError != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantError)):
				test.Errorf("error = %v, want %q", err, testCase.wantError)
			case testCase.wantError == "" && err != nil:
				test.Errorf("unexpected error %v", err)
			}
			if testCase.wantInfo != "" && !contains(inliner.infoMessages(), testCase.wantInfo) {
				test.Errorf("notices = %v, want %q", inliner.infoMessages(), testCase.wantInfo)
			}
			if len(inliner.pullRequestUpdates()) != 0 {
				test.Error("something was sent")
			}
		})
	}
}

func editorTextOfTestDetail() string {
	detail := testDetail()
	return detail.Title + "\n\n" + detail.Body
}

func TestEditPullRequestRefusedWithoutViewerCanUpdate(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Identifier = pullRequestNodeIdentifier
	inliner.client.SetPullRequest(detail) // ViewerCanUpdate false
	inliner.editPullRequest("Other\n\nText")
	if diff := cmp.Diff([]string{noticeCannotEditPullRequest}, inliner.infoMessages()); diff != "" {
		test.Errorf("notices (-want +got):\n%s", diff)
	}
	if len(inliner.openedOn) != 0 {
		test.Error("the editor opened")
	}
}

func TestEditPullRequestShowsGitHubErrors(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("New title\n\nText")
	confirmation, _ := inliner.lastConfirm()
	inliner.client.SetError(fake.MethodUpdatePullRequest, errors.New("boom"))
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if err := inliner.lastErr(); err == nil || !strings.Contains(err.Error(), "boom") {
		test.Errorf("error = %v", err)
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight: %d", inliner.view.writes)
	}
	if inliner.view.detail.Title == "New title" {
		test.Error("the screen shows a title that was not saved")
	}
}

func TestEditPullRequestEditorFailure(test *testing.T) {
	inliner := newInliner(test)
	inliner.err = errors.New("editor died")
	inliner.editPullRequest("ignored")
	if err := inliner.lastErr(); err == nil || !strings.Contains(err.Error(), "editor died") {
		test.Errorf("error = %v", err)
	}
	if _, asked := inliner.lastConfirm(); asked {
		test.Error("a question was asked")
	}
}

func TestEditPullRequestNeverBypassesTheGuard(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.writes = 1
	inliner.editPullRequest("New title\n\nText")
	if diff := cmp.Diff([]string{commentSavingText}, inliner.infoMessages()); diff != "" {
		test.Errorf("notices (-want +got):\n%s", diff)
	}
	if _, ok := inliner.messages[len(inliner.messages)-1].(userinterface.InfoMessage); !ok || len(inliner.openedOn) != 0 {
		test.Error("the command went on")
	}
}

func TestEditPullRequestHostileTitleIsCleaned(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.detail.Title = "evil \x1b]0;pwned\x07 title"
	inliner.view.layout()
	if shown := inliner.render(); strings.Contains(shown, "\x1b]0;") {
		test.Errorf("escape sequence reached the screen: %q", shown)
	}
}
