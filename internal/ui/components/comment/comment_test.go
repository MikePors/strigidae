package comment

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

var now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

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
		if got := RelTime(now, now.Add(-tt.d)); got != tt.want {
			t.Errorf("RelTime(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestHeader(t *testing.T) {
	th := theme.New(true)
	heart := []github.ReactionSummary{{Content: github.ReactionHeart, Count: 2}, {Content: github.ReactionEyes}}
	tests := []struct {
		name string
		c    Comment
		want string
	}{
		{"plain", Comment{Author: "bob", At: now.Add(-5 * time.Minute)}, "bob · 5m ago"},
		{"ghost", Comment{At: now}, "ghost · just now"},
		{"action", Comment{Author: "bob", Action: "commented", At: now}, "bob commented · just now"},
		{"pending comment", Comment{Author: "bob", At: now, Pending: true}, "bob · just now PENDING (draft, not submitted)"},
		{"pending review", Comment{Author: "bob", Action: "review", Pending: true}, "bob PENDING review (draft, not submitted)"},
		{"reactions", Comment{Author: "bob", At: now, Reactions: heart}, "bob · just now  " + github.ReactionHeart.Emoji() + " 2"},
		{"hostile", Comment{Author: "b\x1b[31mob", At: now}, ""},
	}
	for _, tt := range tests {
		got := ansi.Strip(Header(th, now, tt.c))
		if tt.want == "" {
			if got != ansi.Strip(got) || containsEsc(got) {
				t.Errorf("%s: escape in %q", tt.name, got)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func containsEsc(s string) bool {
	for _, r := range s {
		if r == 0x1b {
			return true
		}
	}
	return false
}
