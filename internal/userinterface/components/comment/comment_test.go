package comment

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

var now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func TestRelativeTime(test *testing.T) {
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
	for _, testCase := range tests {
		if got := RelativeTime(now, now.Add(-testCase.d)); got != testCase.want {
			test.Errorf("RelTime(%v) = %q, want %q", testCase.d, got, testCase.want)
		}
	}
}

func TestHeader(test *testing.T) {
	activeTheme := theme.New(true)
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
	for _, testCase := range tests {
		got := ansi.Strip(Header(activeTheme, now, testCase.c))
		if testCase.want == "" {
			if got != ansi.Strip(got) || containsEsc(got) {
				test.Errorf("%s: escape in %q", testCase.name, got)
			}
			continue
		}
		if got != testCase.want {
			test.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
		}
	}
}

func containsEsc(text string) bool {
	for _, character := range text {
		if character == 0x1b {
			return true
		}
	}
	return false
}
