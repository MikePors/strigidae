package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestDescriptionReactions(test *testing.T) {
	testCases := []struct {
		name      string
		reactions []github.ReactionSummary
		want      string
		absent    string
	}{
		{name: "none", reactions: nil, absent: "👍"},
		// ViewerReacted has no visible effect yet (comments do not show it
		// either), so the viewer's own reaction is counted like any other.
		{name: "counts", reactions: []github.ReactionSummary{{Content: github.ReactionThumbsUp, Count: 2, ViewerReacted: true}, {Content: github.ReactionEyes, Count: 1}}, want: "👍 2 👀 1"},
		{name: "zero count hidden", reactions: []github.ReactionSummary{{Content: github.ReactionRocket, Count: 0}}, absent: "🚀"},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			detail := testDetail()
			detail.Body = "Short description."
			detail.Reactions = testCase.reactions
			harness := newHarness(test, 100, 60)
			harness.client.SetPullRequest(detail)
			harness.run(harness.view.Init())
			lines := strings.Split(ansi.Strip(harness.render()), "\n")
			descriptionLine, checksLine := -1, -1
			for i, line := range lines {
				if strings.Contains(line, "Short description.") {
					descriptionLine = i
				}
				if strings.Contains(line, "Checks (") {
					checksLine = i
				}
			}
			if descriptionLine < 0 || checksLine < descriptionLine {
				test.Fatalf("description or checks heading not shown:\n%s", strings.Join(lines, "\n"))
			}
			between := strings.Join(lines[descriptionLine+1:checksLine], "\n")
			if testCase.want != "" && !strings.Contains(lines[descriptionLine+1], testCase.want) {
				test.Errorf("want %q directly under the description, got:\n%s", testCase.want, between)
			}
			if testCase.absent != "" && strings.Contains(between, testCase.absent) {
				test.Errorf("unexpected %q:\n%s", testCase.absent, between)
			}
		})
	}
}

func TestDescriptionReactionsWrapWhole(test *testing.T) {
	var reactions []github.ReactionSummary
	var pairs []string
	for _, content := range []github.ReactionContent{
		github.ReactionThumbsUp, github.ReactionThumbsDown, github.ReactionLaugh, github.ReactionHooray,
		github.ReactionConfused, github.ReactionHeart, github.ReactionRocket, github.ReactionEyes,
	} {
		reactions = append(reactions, github.ReactionSummary{Content: content, Count: 1234})
		pairs = append(pairs, content.Emoji()+" 1234")
	}
	detail := testDetail()
	detail.Body = "Short description."
	detail.Reactions = reactions
	harness := newHarness(test, 60, 60)
	harness.client.SetPullRequest(detail)
	harness.run(harness.view.Init())
	harness.press("l") // narrow: the Overview pane shows only when focused
	out := ansi.Strip(harness.render())
	for _, pair := range pairs {
		if !strings.Contains(out, pair) {
			test.Errorf("reaction %q cut or missing:\n%s", pair, out)
		}
	}
}

func TestConversationOrder(test *testing.T) {
	detail := testDetail()
	got := conversation(&detail)
	var authors []string
	for _, entry := range got {
		authors = append(authors, entry.comment.Author)
	}
	// carol -30h, bob comment -48h => bob, alice(-26h)... sorted by time, pending last.
	want := []string{"bob", "carol", "alice", "bob", "me"}
	if diff := cmp.Diff(want, authors); diff != "" {
		test.Errorf("conversation order (-want +got):\n%s", diff)
	}
	if !got[len(got)-1].comment.Pending {
		test.Error("pending review should be last and flagged")
	}
}

func TestRelativeTimesAge(test *testing.T) {
	now := testNow
	harness := newHarness(test, 100, 60)
	harness.view.now = func() time.Time { return now }
	harness.run(harness.view.Init())
	harness.press("l", "G")
	if !strings.Contains(ansi.Strip(harness.render()), "bob approved · 1h ago") {
		test.Fatal("setup: expected 1h ago")
	}
	now = testNow.Add(2 * time.Hour)
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "bob approved · 3h ago") {
		test.Errorf("relative time did not age:\n%s", out)
	}
}
