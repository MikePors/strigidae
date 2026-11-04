package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func TestCommentsInitialAndNavigatedGroupHeadingsStayVisible(test *testing.T) {
	for _, withDraft := range []bool{true, false} {
		name := "Published"
		if withDraft {
			name = "Drafts (not submitted)"
		}
		test.Run(name, func(test *testing.T) {
			harness := commentsHarness(test, 100, 14)
			if !withDraft {
				var reviews []github.Review
				for _, review := range harness.view.detail.Reviews {
					if review.State != github.ReviewPending {
						reviews = append(reviews, review)
					}
				}
				harness.view.detail.Reviews = reviews
				harness.view.comments.dirty = true
			}
			if !strings.Contains(harness.screen(), name) {
				test.Fatal("initial group heading is off-screen")
			}
			harness.press("G", "g g")
			if !strings.Contains(harness.screen(), name) {
				test.Fatal("gg hid the first group heading")
			}
			harness.press("j", "k")
			if !strings.Contains(harness.screen(), name) {
				test.Fatal("k hid the first group heading")
			}
		})
	}
}

func TestCommentsFirstPublishedConversationShowsHeading(test *testing.T) {
	harness := commentsHarness(test, 100, 14)
	for i, item := range harness.view.comments.items {
		if item.group == 1 {
			harness.view.selectConversation(i, false)
			break
		}
	}
	if !strings.Contains(harness.screen(), "Published") {
		test.Fatal("first published item's heading is off-screen")
	}
}

func TestCommentsHostileCollapsedPreview(test *testing.T) {
	harness := commentsHarness(test, 100, 50)
	const attack = "\x1b[2J"
	harness.view.detail.Comments[0].Body = "preview " + attack + " hostile-marker"
	harness.view.comments.dirty = true
	raw := harness.render()
	if !strings.Contains(ansi.Strip(raw), "hostile-marker") {
		test.Fatal("hostile collapsed preview was not visible")
	}
	if strings.Contains(raw, attack) {
		test.Fatal("raw terminal erase escaped through collapsed preview")
	}
}

func TestCommentsHostileOriginalContext(test *testing.T) {
	harness := commentsHarness(test, 100, 24)
	const attack = "\x1b[2J"
	for i := range harness.view.threads {
		if harness.view.threads[i].Identifier == "t-outdated" {
			harness.view.threads[i].Comments[0].DiffHunk = "@@ old context @@\n+ " + attack + "old-context-marker"
		}
	}
	harness.view.comments.dirty = true
	selectCommentThread(test, harness, "t-outdated")
	raw := harness.render()
	plain := ansi.Strip(raw)
	if !strings.Contains(plain, "Original context:") || !strings.Contains(plain, "old-context-marker") {
		test.Fatal("hostile original context was not visible")
	}
	if strings.Contains(raw, attack) {
		test.Fatal("raw terminal erase escaped through original context")
	}
}

func TestCommentsDraftCountIncludesReviewSummary(test *testing.T) {
	harness := commentsHarness(test, 100, 24)
	if harness.view.comments.drafts != 1 || !strings.Contains(harness.screen(), "1 draft comments") {
		test.Fatal("pending review body was not counted")
	}
	harness.view.threads[0].Comments[0].Pending = true
	harness.view.comments.dirty = true
	harness.view.syncComments()
	if harness.view.comments.drafts != 2 {
		test.Fatal("pending review body and thread comment were not both counted")
	}
}

func TestCommentsNarrowNoticesDistinguishDiffActions(test *testing.T) {
	for _, pressed := range []string{"e", "D", "c", "m"} {
		test.Run(pressed, func(test *testing.T) {
			harness := commentsHarness(test, 60, 20)
			harness.press("h", pressed)
			if !strings.HasPrefix(harness.lastInfo(), "focus the diff to ") {
				test.Fatalf("misleading notice: %q", harness.lastInfo())
			}
		})
	}
	harness := commentsHarness(test, 60, 20)
	harness.press("h")
	command, _ := harness.view.Command("react", "heart")
	harness.run(command)
	if !strings.HasPrefix(harness.lastInfo(), "focus the diff to ") {
		test.Fatalf("misleading reaction notice: %q", harness.lastInfo())
	}
}

func TestCommentsJumpSurvivesUnrelatedOptions(test *testing.T) {
	harness := commentsHarness(test, 100, 24)
	values := options.Values{options.HideResolved: true}
	harness.send(userinterface.OptionsMessage{Values: values})
	selectCommentThread(test, harness, "t-resolved")
	harness.press("enter")
	values = options.Values{options.HideResolved: true, options.Wrap: true}
	harness.send(userinterface.OptionsMessage{Values: values})
	context := harness.view.filePane.Context()
	if len(context.Threads) != 1 || context.Threads[0].Identifier != "t-resolved" {
		test.Fatal("unrelated wrap change hid the jumped-to thread")
	}
}

func TestCommentsJumpSurvivesEarlierHunkFade(test *testing.T) {
	hidden := newHideHarness(test, false)
	hidden.client.SetReviewThreads(testRepository, 7, testThreads())
	hidden.press("R")
	hidden.send(userinterface.OptionsMessage{Values: options.Values{options.HideReviewed: true}})
	hidden.press("}", "l", "m", "]", "m")
	hidden.run(hidden.view.openComments(""))
	selectCommentThread(test, hidden.harness, "t-open")
	hidden.press("enter")
	hidden.fire(fadeDuration)
	context := hidden.view.filePane.Context()
	if len(context.Threads) != 1 || context.Threads[0].Identifier != "t-open" {
		test.Fatal("an earlier hunk fade hid the newly revealed jump target")
	}
}
