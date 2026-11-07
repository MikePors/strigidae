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
		duration time.Duration
		want     string
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
		if got := RelativeTime(now, now.Add(-testCase.duration)); got != testCase.want {
			test.Errorf("RelativeTime(%v) = %q, want %q", testCase.duration, got, testCase.want)
		}
	}
}

func TestHeader(test *testing.T) {
	activeTheme := theme.New(true)
	heart := []github.ReactionSummary{{Content: github.ReactionHeart, Count: 2}, {Content: github.ReactionEyes}}
	tests := []struct {
		name    string
		comment Comment
		want    string
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
		got := ansi.Strip(Header(activeTheme, now, testCase.comment))
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

func TestReactionTextStylesOwnReactions(test *testing.T) {
	activeTheme := theme.New(true)
	own := activeTheme.OwnReaction.Render
	other := activeTheme.Muted.Render
	reactionList := []github.ReactionSummary{
		{Content: github.ReactionThumbsUp, Count: 3, ViewerReacted: true},
		{Content: github.ReactionLaugh, Count: 1},
		{Content: github.ReactionRocket},
		{Content: github.ReactionHeart, Count: 2, ViewerReacted: true},
	}
	thumbsUp, laugh, heart := github.ReactionThumbsUp.Emoji()+" 3", github.ReactionLaugh.Emoji()+" 1", github.ReactionHeart.Emoji()+" 2"
	tests := []struct {
		name  string
		width int
		want  string
	}{
		{"one line", 0, own(thumbsUp) + " " + other(laugh) + " " + own(heart)},
		// "👍 3 😄 1" is 9 columns, so a width of 9 breaks only before the heart.
		{"wrapped", 9, own(thumbsUp) + " " + other(laugh) + "\n" + own(heart)},
	}
	for _, testCase := range tests {
		if got := ReactionText(activeTheme, reactionList, testCase.width); got != testCase.want {
			test.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
		}
	}
	if activeTheme.OwnReaction.Render("x") == activeTheme.Muted.Render("x") {
		test.Error("own reactions look the same as the others")
	}
}
