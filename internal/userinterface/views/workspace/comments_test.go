package workspace

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func commentsHarness(test *testing.T, width, height int) *harness {
	test.Helper()
	harness := threadHarness(test, width, height)
	// Real GitHub items have stable identifiers, unlike the older Overview fixtures.
	for i := range harness.view.detail.Comments {
		harness.view.detail.Comments[i].Identifier = fmt.Sprintf("comment-%d", i)
	}
	for i := range harness.view.detail.Reviews {
		harness.view.detail.Reviews[i].Identifier = fmt.Sprintf("review-%d", i)
	}
	harness.view.comments.dirty = true
	command, handled := harness.view.Command("comments", "")
	if !handled {
		test.Fatal("comments command was not registered")
	}
	harness.run(command)
	return harness
}

func selectCommentThread(test *testing.T, harness *harness, identifier string) {
	test.Helper()
	harness.view.syncComments()
	row := slices.IndexFunc(harness.view.comments.items, func(item conversationItem) bool { return item.identifier == "thread:"+identifier })
	if row < 0 {
		test.Fatalf("thread %s missing", identifier)
	}
	harness.view.selectConversation(row, false)
}

func TestCommentsSidebarAndNarrowFocus(test *testing.T) {
	harness := loaded(test, 60, 20)
	harness.press("j")
	if harness.view.mode != contentComments || harness.view.selection() != 0 {
		test.Fatal("Comments treated as a diff file")
	}
	harness.press("enter")
	if harness.view.focus != paneRight || !strings.Contains(harness.screen(), "Published") {
		test.Fatal("Comments did not take narrow focus")
	}
	harness.press("h", "j")
	if harness.view.selectedPath() != firstFile {
		test.Fatal("file after Comments was not selected")
	}
	harness.press("{")
	if harness.view.selectedPath() != firstFile {
		test.Fatal("previous-file landed on Comments")
	}
}

func TestCommentsIndexAndFilter(test *testing.T) {
	harness := commentsHarness(test, 100, 30)
	pane := &harness.view.comments
	if pane.total != 9 || pane.open != 3 || len(pane.items) != 9 {
		test.Fatalf("counts: total %d, open %d, items %d", pane.total, pane.open, len(pane.items))
	}
	if !pane.items[0].comment.Pending {
		test.Fatal("draft review not grouped first")
	}
	harness.run(harness.view.openComments("unresolved"))
	if len(pane.items) != 3 {
		test.Fatalf("unresolved items = %d", len(pane.items))
	}
	for _, item := range pane.items {
		if item.thread == nil || item.thread.IsResolved {
			test.Fatal("non-unresolved item in filter")
		}
	}
	selectCommentThread(test, harness, "t-open")
	harness.press("R")
	harness.view.syncComments()
	if harness.view.mode != contentComments || pane.selected != "thread:t-open" {
		test.Fatal("refresh lost Comments selection")
	}
}

func TestCommentsSearchHiddenReplyAndSeparateQueries(test *testing.T) {
	harness := commentsHarness(test, 100, 24)
	harness.press("/")
	harness.searchFor("flaky", userinterface.PromptSubmitted)
	if harness.view.comments.selected != "thread:t-open" {
		test.Fatal("search did not select collapsed reply")
	}
	if !strings.Contains(harness.screen(), "Handles flaky links.") {
		test.Fatal("matching reply is not visible")
	}
	if harness.view.search.Active() {
		test.Fatal("Comments search leaked into Files")
	}
	harness.press("esc")
	if harness.view.comments.search.Active() {
		test.Fatal("esc did not clear Comments search")
	}
	harness.run(harness.view.openComments("all"))
	harness.press("/")
	harness.searchFor("outdated", userinterface.PromptSubmitted)
	if harness.view.comments.selected != "thread:t-outdated" {
		test.Fatal("outdated state not searchable")
	}
}

func TestCommentsJumpAndReturn(test *testing.T) {
	for _, identifier := range []string{"t-open", "t-resolved"} {
		test.Run(identifier, func(test *testing.T) {
			harness := commentsHarness(test, 100, 24)
			harness.view.detail.Files[0].Viewed = github.FileViewed
			harness.send(userinterface.OptionsMessage{Values: options.Values{options.HideViewed: true, options.HideResolved: true, options.HideReviewed: true}})
			selectCommentThread(test, harness, identifier)
			harness.press("enter")
			context := harness.view.filePane.Context()
			if harness.view.selectedPath() != firstFile || len(context.Threads) != 1 || context.Threads[0].Identifier != identifier {
				test.Fatalf("jump selected %s / %#v", harness.view.selectedPath(), context.Threads)
			}
			harness.run(harness.view.openComments(""))
			if harness.view.comments.selected != "thread:"+identifier {
				test.Fatal("return lost selection")
			}
		})
	}
}

func TestCommentsOutdatedAndMissingAnchorStayPut(test *testing.T) {
	for _, identifier := range []string{"t-outdated", "t-readme"} {
		test.Run(identifier, func(test *testing.T) {
			harness := commentsHarness(test, 100, 24)
			selectCommentThread(test, harness, identifier)
			harness.press("enter")
			if harness.view.mode != contentComments {
				test.Fatal("missing anchor opened unrelated code")
			}
			if !strings.Contains(harness.lastInfo(), "No current diff location") {
				test.Fatal("missing anchor notice not shown")
			}
		})
	}
}

func TestCommentsResolveUsesSelectedThreadAndKeepsConfirmation(test *testing.T) {
	harness := commentsHarness(test, 100, 24)
	harness.run(harness.view.openComments("unresolved"))
	selectCommentThread(test, harness, "t-open")
	harness.press("x")
	var confirmation *userinterface.ConfirmMessage
	for _, message := range harness.messages {
		if question, ok := message.(userinterface.ConfirmMessage); ok {
			confirmation = &question
		}
	}
	if confirmation == nil {
		test.Fatalf("no confirmation: %#v", harness.messages)
	}
	// Moving selection after asking must not retarget the action.
	selectCommentThread(test, harness, "t-outdated")
	harness.run(confirmation.OnYes)
	harness.view.syncComments()
	for _, item := range harness.view.comments.items {
		if item.identifier == "thread:t-open" {
			test.Fatal("resolved thread remains in unresolved filter")
		}
	}
	for _, reviewThread := range harness.view.threads {
		if reviewThread.Identifier == "t-outdated" && reviewThread.IsResolved {
			test.Fatal("resolution retargeted cursor")
		}
	}
}

func TestCommentsPartialFailureRetainsItems(test *testing.T) {
	harness := commentsHarness(test, 100, 30)
	selectCommentThread(test, harness, "t-open")
	harness.client.SetError(fake.MethodListReviewThreads, errors.New("offline"))
	harness.client.SetError(fake.MethodGetPullRequest, errors.New("offline"))
	harness.press("R")
	screen := harness.screen()
	if !strings.Contains(screen, "could not be loaded") || !strings.Contains(screen, "could not be refreshed") || !strings.Contains(screen, "Why retry here?") {
		test.Fatalf("partial failure lost content or notice:\n%s", screen)
	}
}

func TestCommentsGolden(test *testing.T) {
	for _, testCase := range []struct {
		name          string
		width, height int
		setup         func(*harness)
	}{
		{"comments", 100, 34, func(harness *harness) { selectCommentThread(test, harness, "t-open") }},
		{"comments_narrow", 60, 20, func(harness *harness) { selectCommentThread(test, harness, "t-open") }},
		{"comments_empty", 100, 20, func(harness *harness) {
			harness.view.detail.Comments = nil
			harness.view.detail.Reviews = nil
			harness.view.threads = nil
			harness.view.comments.dirty = true
		}},
		{"comments_filtered_empty", 100, 20, func(harness *harness) {
			harness.view.threads = nil
			harness.run(harness.view.openComments("unresolved"))
		}},
		{"comments_error", 100, 20, func(harness *harness) { harness.view.threadErr = errors.New("offline") }},
		{"comments_loading", 100, 20, func(harness *harness) { harness.view.threadPending = true }},
		{"comments_long", 100, 20, func(harness *harness) {
			harness.view.threads[3].Comments[1].Body = strings.Repeat("A long reply.\n\n", 30)
			harness.view.comments.dirty = true
			selectCommentThread(test, harness, "t-open")
			harness.press("ctrl+d")
		}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			harness := commentsHarness(test, testCase.width, testCase.height)
			testCase.setup(harness)
			golden.Assert(test, testCase.name, harness.render(), false)
		})
	}
}

func TestCommentsHostileTextAndDrafts(test *testing.T) {
	harness := commentsHarness(test, 100, 30)
	harness.view.threads = append(harness.view.threads, github.ReviewThread{Identifier: "draft", Path: "evil\x1b[2J.go", Line: 1, Comments: []github.ReviewComment{{Author: "me", Body: "draft\x1b[2J content", Pending: true, CreatedAt: testNow.Add(time.Hour)}}})
	harness.view.comments.dirty = true
	harness.view.syncComments()
	selectCommentThread(test, harness, "draft")
	raw := harness.render()
	screen := ansi.Strip(raw)
	if strings.Contains(raw, "\x1b[2J") || !strings.Contains(screen, "draft") || harness.view.comments.drafts != 1 {
		test.Fatalf("draft or text sanitization failed:\n%s", screen)
	}
}

func TestCommentsReplyUsesSelectedThread(test *testing.T) {
	composer := newComposer(test)
	composer.run(composer.view.openComments(""))
	selectCommentThread(test, composer.harness, "t-open")
	composer.body = "Thanks, fixed."
	composer.press("r")
	confirmation, ok := composer.lastConfirm()
	if !ok || len(composer.openedOn) != 1 {
		test.Fatal("reply did not open editor and ask for confirmation")
	}
	selectCommentThread(test, composer.harness, "t-outdated")
	composer.run(confirmation.OnYes)
	replies := composer.client.Replies()
	if len(replies) != 1 || replies[0].ThreadIdentifier != "t-open" {
		test.Fatalf("reply targeted %#v", replies)
	}
}

func TestCommentsLocalResultsAreSeparateAndReadOnly(test *testing.T) {
	harness := commentsHarness(test, 100, 24)
	harness.send(userinterface.LocalThreadsMessage{Threads: []userinterface.LocalThread{{
		Identifier: "local-result", Repository: testRepository, Number: 7, Head: harness.view.diffHead,
		Path: firstFile, Line: 14, Side: github.SideRight, Question: "Why here?", Text: "Local answer", State: github.LocalAnswered,
	}}})
	selectCommentThread(test, harness, "local-result")
	if pane := &harness.view.comments; pane.total != 9 || pane.open != 3 || len(pane.items) != 10 {
		test.Fatalf("local result altered GitHub counts: %d / %d / %d", pane.total, pane.open, len(pane.items))
	}
	for _, pressed := range []string{"r", "x"} {
		harness.press(pressed)
		if !strings.Contains(harness.lastInfo(), "local to strig") {
			test.Fatalf("%s did not refuse local write", pressed)
		}
	}
	harness.press("o")
	if len(harness.opened) != 0 {
		test.Fatal("local result opened a GitHub URL")
	}
	golden.Assert(test, "comments_local", harness.render(), false)
}

func TestCommentsJumpRefusesLoadingAndStaleDiff(test *testing.T) {
	for _, testCase := range []struct {
		name   string
		change func(*View)
	}{
		{"loading", func(view *View) { view.diffPending = true }},
		{"stale", func(view *View) { view.diffHead = "old"; view.detail.HeadSHA = "new" }},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			harness := commentsHarness(test, 100, 24)
			selectCommentThread(test, harness, "t-open")
			testCase.change(harness.view)
			harness.press("enter")
			if harness.view.mode != contentComments || harness.lastInfo() == "" {
				test.Fatal("jump used a diff that is not current")
			}
		})
	}
}
