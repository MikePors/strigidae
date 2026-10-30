package workspace

import (
	"strings"
	"testing"
)

func TestEditPullRequestShowsTheNewTextBeforeTheReload(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("New title\n\nNew body")
	confirmation, _ := inliner.lastConfirm()
	agreed, ok := confirmation.OnYes().(commentAgreedMessage)
	if !ok {
		test.Fatal("the yes did not start the write")
	}
	_, send := inliner.view.Update(agreed)
	_, reload := inliner.view.Update(send())
	if reload == nil {
		test.Fatal("no reload after the update")
	}
	// The reload command has not run, so this is the update's own doing.
	if inliner.view.detail.Title != "New title" || inliner.view.detail.Body != "New body" {
		test.Errorf("detail = %q, %q before the reload", inliner.view.detail.Title, inliner.view.detail.Body)
	}
}

func TestEditPullRequestIgnoresResultsOfAnEarlierFlow(test *testing.T) {
	test.Run("the re-read", func(test *testing.T) {
		inliner := newInliner(test)
		read, _ := inliner.view.Command("edit-pull-request", "")
		message := read()
		inliner.view.commentSequence++ // another editor started meanwhile
		inliner.openedOn = nil
		_, command := inliner.view.Update(message)
		if command != nil || len(inliner.openedOn) != 0 {
			test.Error("a stale re-read opened the editor")
		}
	})
	test.Run("the editor's text", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.body = "New title\n\nNew body"
		read, _ := inliner.view.Command("edit-pull-request", "")
		_, opened := inliner.view.Update(read())
		message := opened()
		inliner.view.commentSequence++
		inliner.messages = nil
		_, command := inliner.view.Update(message)
		inliner.run(command)
		if _, asked := inliner.lastConfirm(); asked {
			test.Error("a stale editor result asked a question")
		}
	})
}

func TestEditPullRequestHostileTextIsCleaned(test *testing.T) {
	inliner := newInliner(test)
	inliner.editPullRequest("evil \x1b]0;pwned\x07 title\n\nbody \x1b]0;pwned\x07 \u202e text")
	confirmation, _ := inliner.lastConfirm()
	inliner.run(confirmation.OnYes)
	inliner.press("h", "g g") // the Overview
	if shown := inliner.render(); strings.Contains(shown, "\x1b]0;") || strings.Contains(shown, "\u202e") {
		test.Errorf("a control sequence reached the screen: %q", shown)
	}
	if !strings.Contains(inliner.shown(), "evil") {
		test.Error("the new title is not shown")
	}
}
