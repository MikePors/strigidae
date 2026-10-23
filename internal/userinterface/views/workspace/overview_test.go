package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

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
	harness.v.now = func() time.Time { return now }
	harness.run(harness.v.Init())
	harness.press("l", "G")
	if !strings.Contains(ansi.Strip(harness.render()), "bob approved · 1h ago") {
		test.Fatal("setup: expected 1h ago")
	}
	now = testNow.Add(2 * time.Hour)
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "bob approved · 3h ago") {
		test.Errorf("relative time did not age:\n%s", out)
	}
}
