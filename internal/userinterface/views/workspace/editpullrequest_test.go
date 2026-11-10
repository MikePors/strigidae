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

// changeOnGitHub replaces the pull request on the server, as a collaborator
// editing it on github.com would.
func (inliner *inliner) changeOnGitHub(title, body string) {
	detail := testDetail()
	detail.Identifier, detail.ViewerCanUpdate = pullRequestNodeIdentifier, true
	detail.Title, detail.Body = title, body
	inliner.client.SetPullRequest(detail)
}

func TestEditPullRequestAsksAgainAfterAConcurrentChange(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("My title\n\nMy body")
	confirmation, _ := inliner.lastConfirm()
	inliner.changeOnGitHub("Their title", "Their body")
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if len(inliner.pullRequestUpdates()) != 0 {
		test.Fatal("overwrote the collaborator's change")
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight while the question is open: %d", inliner.view.writes)
	}
	again, asked := inliner.lastConfirm()
	if !asked || !strings.Contains(again.Text, "changed on GitHub") {
		test.Fatalf("second question = %+v", again)
	}

	// Yes to the conflict writes the user's edit over the newer text.
	inliner.run(again.OnYes)
	want := []github.UpdatePullRequestInput{{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Title: "My title", Body: "My body"}}
	if diff := cmp.Diff(want, inliner.pullRequestUpdates()); diff != "" {
		test.Errorf("update (-want +got):\n%s", diff)
	}
}

func TestEditPullRequestChangedAgainWhileAskingAgain(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("My title\n\nMy body")
	confirmation, _ := inliner.lastConfirm()
	inliner.changeOnGitHub("Their title", "Their body")
	inliner.run(confirmation.OnYes)
	again, _ := inliner.lastConfirm()
	inliner.changeOnGitHub("Their title", "Their body, edited again")
	inliner.messages = nil
	inliner.run(again.OnYes)
	if len(inliner.pullRequestUpdates()) != 0 {
		test.Fatal("wrote on a yes that was about older text")
	}
	if third, asked := inliner.lastConfirm(); !asked || !strings.Contains(third.Text, "changed on GitHub") {
		test.Errorf("no third question; messages %v", inliner.messages)
	}
}

func TestEditPullRequestNoLongerAllowedAfterTheYes(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("My title\n\nMy body")
	confirmation, _ := inliner.lastConfirm()
	detail := testDetail()
	detail.Identifier = pullRequestNodeIdentifier // ViewerCanUpdate false
	inliner.client.SetPullRequest(detail)
	inliner.messages = nil
	before := len(inliner.client.Calls())
	inliner.run(confirmation.OnYes)
	if len(inliner.pullRequestUpdates()) != 0 {
		test.Error("something was sent")
	}
	if got := inliner.infoMessages(); !contains(got, noticeCannotEditPullRequest) {
		test.Errorf("notices = %v, want %q", got, noticeCannotEditPullRequest)
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight: %d", inliner.view.writes)
	}
	var reloaded bool
	for _, call := range inliner.client.Calls()[before:] {
		reloaded = reloaded || call.Method == fake.MethodGetPullRequest
	}
	if !reloaded {
		test.Error("the pull request was not reloaded")
	}
}

func TestEditPullRequestDecliningTheConflictWritesNothing(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("My title\n\nMy body")
	confirmation, _ := inliner.lastConfirm()
	inliner.changeOnGitHub("Their title", "Their body")
	inliner.run(confirmation.OnYes)
	if _, asked := inliner.lastConfirm(); !asked {
		test.Fatal("no conflict question")
	}
	// Declining is the shell dropping OnYes.
	if len(inliner.pullRequestUpdates()) != 0 || inliner.view.writes != 0 {
		test.Errorf("updates = %v, writes in flight: %d", inliner.pullRequestUpdates(), inliner.view.writes)
	}
	if inliner.view.detail.Title == "My title" {
		test.Error("the screen shows a title that was not saved")
	}
}

func TestEditPullRequestReadFailsAfterTheYes(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("My title\n\nMy body")
	confirmation, _ := inliner.lastConfirm()
	inliner.client.SetError(fake.MethodGetPullRequest, errors.New("502 Bad Gateway"))
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if len(inliner.pullRequestUpdates()) != 0 {
		test.Error("wrote without re-reading")
	}
	if err := inliner.lastErr(); err == nil || !strings.Contains(err.Error(), "read pull request") {
		test.Errorf("error = %v", err)
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight: %d", inliner.view.writes)
	}
}
