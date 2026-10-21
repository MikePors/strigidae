package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

func TestConversationOrder(t *testing.T) {
	d := testDetail()
	got := conversation(&d)
	var authors []string
	for _, e := range got {
		authors = append(authors, e.author)
	}
	// carol -30h, bob comment -48h => bob, alice(-26h)... sorted by time, pending last.
	want := []string{"bob", "carol", "alice", "bob", "me"}
	if diff := cmp.Diff(want, authors); diff != "" {
		t.Errorf("conversation order (-want +got):\n%s", diff)
	}
	if !got[len(got)-1].pending {
		t.Error("pending review should be last and flagged")
	}
}

func TestRelTime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "just now"},
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{3 * time.Hour, "3h ago"},
		{50 * time.Hour, "2d ago"},
		{75 * 24 * time.Hour, "2mo ago"},
		{800 * 24 * time.Hour, "2y ago"},
	}
	for _, tt := range tests {
		if got := relTime(testNow, testNow.Add(-tt.d)); got != tt.want {
			t.Errorf("relTime(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestRelativeTimesAge(t *testing.T) {
	now := testNow
	h := newHarness(t, 100, 60)
	h.v.now = func() time.Time { return now }
	h.run(h.v.Init())
	h.press("l", "G")
	if !strings.Contains(ansi.Strip(h.render()), "bob approved · 1h ago") {
		t.Fatal("setup: expected 1h ago")
	}
	now = testNow.Add(2 * time.Hour)
	if out := ansi.Strip(h.render()); !strings.Contains(out, "bob approved · 3h ago") {
		t.Errorf("relative time did not age:\n%s", out)
	}
}
