package workspace

import (
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface"
)

func TestCommentsSearchRevealsMatchingReply(test *testing.T) {
	for _, testCase := range []struct{ name, body, query string }{
		{"paragraphs", "needle\n\ntarget", "needle target"},
		{"wrapped", "needle " + strings.Repeat("padding ", 12) + "target", "needle target"},
		{"author and body", "needle\n\ntarget", "alice needle target"},
		{"markdown source", "**needle** **target**", "**needle** **target**"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			harness := commentsHarness(test, 100, 14)
			for i := range harness.view.threads {
				if harness.view.threads[i].Identifier == "t-open" {
					harness.view.threads[i].Comments[0].Body = strings.Repeat("Long initial comment.\n\n", 50)
					harness.view.threads[i].Comments[1].Body = testCase.body
				}
			}
			harness.view.comments.dirty = true
			harness.view.syncComments()
			harness.press("/")
			harness.searchFor(testCase.query, userinterface.PromptSubmitted)
			if harness.view.comments.selected != "thread:t-open" {
				test.Fatal("conversation not selected")
			}
			if !strings.Contains(harness.screen(), "target") {
				test.Fatal("matching reply is off screen after successful search")
			}
		})
	}
}

func TestCommentsSearchAfterThreadsArriveBeforeRender(test *testing.T) {
	harness := commentsHarness(test, 100, 20)
	harness.press("/")
	threads := testThreads()
	threads[0].Comments[1].Body = "arriving reply"
	harness.send(threadsLoadedMessage{owner: harness.view, repository: testRepository, number: 7, sequence: harness.view.sequence, generation: harness.view.threadGeneration, threads: threads})
	harness.searchFor("arriving", userinterface.PromptChanged)
	if harness.view.comments.selected != "thread:t-open" {
		test.Fatalf("selection %q; query used stale index", harness.view.comments.selected)
	}
}
