package workspace

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func TestCommentsFallbackAfterResolvingLastThreadShowsHeader(test *testing.T) {
	harness := commentsHarness(test, 100, 16)
	var threads []github.ReviewThread
	for i := range 6 {
		threads = append(threads, github.ReviewThread{
			Identifier: fmt.Sprintf("thread-%d", i), Path: firstFile, Line: 14, DiffSide: github.SideRight,
			Comments: []github.ReviewComment{newComment(fmt.Sprintf("reviewer-%d", i), strings.Repeat("Long conversation.\n\n", 20), time.Duration(i+1)*time.Hour)},
		})
	}
	harness.client.SetReviewThreads(testRepository, 7, threads)
	harness.press("R")
	harness.run(harness.view.openComments("unresolved"))
	harness.press("G")
	harness.render()
	harness.press("x")
	composer := &composer{harness: harness}
	question, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("resolve did not ask for confirmation")
	}
	harness.run(question.OnYes)
	screen := harness.screen()
	if harness.view.comments.selected != "thread:thread-4" {
		test.Fatalf("fallback selected %s", harness.view.comments.selected)
	}
	if !strings.Contains(screen, "> ▾") || !strings.Contains(screen, "reviewer-4") {
		test.Fatalf("fallback selection's header is off-screen:\n%s", screen)
	}
}

func TestCommentsSelectionFollowsIdentityWhenRowsShift(test *testing.T) {
	original := testThreads()
	extra := github.ReviewThread{Identifier: "newest", Path: firstFile, Line: 14, Comments: []github.ReviewComment{newComment("new-reviewer", "New conversation", -time.Hour)}}
	inserted := append(append([]github.ReviewThread{}, original...), extra)
	for _, testCase := range []struct {
		name          string
		before, after []github.ReviewThread
		delta         int
	}{
		{"insert above", original, inserted, 1},
		{"remove above", inserted, original, -1},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			harness := commentsHarness(test, 100, 18)
			harness.client.SetReviewThreads(testRepository, 7, testCase.before)
			harness.press("R")
			selectCommentThread(test, harness, "t-open")
			before := harness.view.comments.window.Cursor()
			harness.client.SetReviewThreads(testRepository, 7, testCase.after)
			harness.press("R")
			harness.render()
			pane := &harness.view.comments
			want := before + testCase.delta
			if pane.selected != "thread:t-open" || pane.window.Cursor() != want || pane.items[pane.window.Cursor()].identifier != "thread:t-open" {
				test.Fatalf("refresh selection %q at row %d, want t-open at %d", pane.selected, pane.window.Cursor(), want)
			}
		})
	}
}

func TestCommentsOpenSelectedGitHubPermalink(test *testing.T) {
	for _, kind := range []string{"comment", "review", "thread"} {
		test.Run(kind, func(test *testing.T) {
			harness := commentsHarness(test, 100, 24)
			address := "https://example.test/pull/7#selected-" + kind
			switch kind {
			case "comment":
				harness.view.detail.Comments[0].URL = address
			case "review":
				harness.view.detail.Reviews[0].URL = address
			case "thread":
				harness.view.threads[3].Comments[0].URL = address
			}
			harness.view.comments.dirty = true
			harness.view.syncComments()
			found := false
			for i, item := range harness.view.comments.items {
				if item.URL == address {
					harness.view.selectConversation(i, false)
					found = true
					break
				}
			}
			if !found {
				test.Fatal("permalink missing from index")
			}
			harness.press("o")
			if len(harness.opened) != 1 || harness.opened[0] != address {
				test.Fatalf("opened %v, want %s", harness.opened, address)
			}
		})
	}
}

func TestCommentsTopLevelCommentKeepsConfirmation(test *testing.T) {
	composer := newComposer(test)
	composer.run(composer.view.openComments(""))
	composer.body = "PR-wide response"
	composer.press("C")
	question, ok := composer.lastConfirm()
	if !ok || len(composer.openedOn) != 1 {
		test.Fatal("C did not open editor and ask for confirmation")
	}
	if len(composer.client.Comments()) != 0 {
		test.Fatal("C posted before confirmation")
	}
	composer.run(question.OnYes)
	comments := composer.client.Comments()
	if len(comments) != 1 || comments[0].Body != "PR-wide response" {
		test.Fatalf("posted %#v", comments)
	}
	if composer.view.mode != contentComments {
		test.Fatal("posting left Comments")
	}
}

func TestCommentsRejectsBadArgumentsWithoutChangingView(test *testing.T) {
	harness := loaded(test, 100, 24)
	command, handled := harness.view.Command("comments", "invalid")
	if !handled {
		test.Fatal("comments command not recognized")
	}
	harness.run(command)
	if err := harness.lastErr(); err == nil || err.Error() != "usage: :comments [all|unresolved]" {
		test.Fatalf("bad argument error: %v", err)
	}
	if harness.view.mode != contentOverview || harness.view.focus != paneFiles {
		test.Fatal("invalid arguments changed the view")
	}
}

func TestCommentsSearchThenOtherPaneSearchUsesThatPane(test *testing.T) {
	for _, destination := range []string{"Files", "diff"} {
		test.Run(destination, func(test *testing.T) {
			harness := commentsHarness(test, 100, 24)
			harness.press("/")
			harness.searchFor("flaky", userinterface.PromptSubmitted)
			if destination == "Files" {
				harness.press("h")
			} else {
				harness.press("}")
			}
			harness.press("/")
			query := "context"
			if destination == "Files" {
				query = "retry"
			}
			harness.searchFor(query, userinterface.PromptSubmitted)
			if harness.view.comments.search.Query() != "flaky" {
				test.Fatal("new pane's query replaced Comments query")
			}
			if destination == "Files" {
				if harness.view.search.Query() != query || harness.view.selectedPath() != "internal/sync/retry.go" {
					test.Fatal("Files search was routed to Comments")
				}
			} else if len(harness.diffMarked()) == 0 {
				test.Fatal("diff search was routed to Comments")
			}
		})
	}
}

func TestCommentsNarrowThreadActionsNameHiddenPane(test *testing.T) {
	harness := commentsHarness(test, 60, 20)
	selectCommentThread(test, harness, "t-open")
	harness.press("h", "r")
	if got := harness.lastInfo(); got != "focus Comments to reply" {
		test.Fatalf("notice %q", got)
	}
}

func TestCommentsEmptyReviewHasOneSeparator(test *testing.T) {
	harness := commentsHarness(test, 100, 50)
	harness.view.comments.viewport.GotoTop()
	screen := ansi.Strip(strings.Join(harness.view.renderComments(70, 46), "\n"))
	lines := strings.Split(screen, "\n")
	found := false
	for i, line := range lines {
		if !strings.Contains(line, "bob approved") {
			continue
		}
		found = true
		if i+2 >= len(lines) || strings.TrimSpace(lines[i+1]) != "" || strings.TrimSpace(lines[i+2]) == "" {
			test.Fatal("verdict without body should have exactly one separator")
		}
	}
	if !found {
		test.Fatal("review verdict was not rendered")
	}
}
