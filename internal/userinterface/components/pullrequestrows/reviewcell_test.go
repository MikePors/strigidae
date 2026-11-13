package pullrequestrows

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestReviewCell(test *testing.T) {
	activeTheme := theme.Default()
	styles := activeTheme.PullRequestList
	table := New(activeTheme, time.Now, Options{})
	const head, older = "aaaa", "bbbb"
	tests := []struct {
		name        string
		pullRequest github.PullRequest
		want        cell
	}{
		{"never reviewed", github.PullRequest{HeadSHA: head}, cell{{text: " ", style: styles.Comments}}},
		{"reviewed at head", github.PullRequest{HeadSHA: head, ViewerReviewedSHA: head}, cell{{text: " ", style: styles.Comments}}},
		{
			"reviewed at an older commit",
			github.PullRequest{HeadSHA: head, ViewerReviewedSHA: older},
			cell{{text: " ", style: styles.Comments}, {text: "+", style: styles.NewCommits}},
		},
		{
			"approved at an older commit",
			github.PullRequest{HeadSHA: head, ViewerReviewedSHA: older, ReviewDecision: github.ReviewDecisionApproved},
			cell{{text: "✓", style: styles.Approved}, {text: "+", style: styles.NewCommits}},
		},
		{
			"approved at head",
			github.PullRequest{HeadSHA: head, ViewerReviewedSHA: head, ReviewDecision: github.ReviewDecisionApproved},
			cell{{text: "✓", style: styles.Approved}},
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got := table.reviewCell(testCase.pullRequest)
			if diff := cmp.Diff(segmentTexts(testCase.want), segmentTexts(got)); diff != "" {
				test.Fatalf("review cell text mismatch (-want +got):\n%s", diff)
			}
			for i := range got {
				if got[i].style.Render("x") != testCase.want[i].style.Render("x") {
					test.Errorf("segment %d is styled differently from the want", i)
				}
			}
		})
	}
}

// TestNewCommitsGlyphKeepsTheColumnWidth checks that the glyph replaces a blank
// in the review column without widening the row or shifting the columns after
// it, selected or not.
func TestNewCommitsGlyphKeepsTheColumnWidth(test *testing.T) {
	table := New(theme.Default(), time.Now, Options{})
	reviewed := github.PullRequest{Number: 7, Title: "Title", Author: "alice", HeadSHA: "a", ViewerReviewedSHA: "b", CommentCount: 3}
	unchanged := reviewed
	unchanged.ViewerReviewedSHA = "a"
	const width = 60
	layout := table.Layout([]github.PullRequest{reviewed}, width-MarkerColumns)
	for _, selected := range []bool{false, true} {
		withGlyph := ansi.Strip(table.Row(layout, reviewed, selected, false, width))
		withoutGlyph := ansi.Strip(table.Row(layout, unchanged, selected, false, width))
		if ansi.StringWidth(withGlyph) != width || ansi.StringWidth(withoutGlyph) != width {
			test.Errorf("selected=%v: widths %d and %d, want %d", selected, ansi.StringWidth(withGlyph), ansi.StringWidth(withoutGlyph), width)
		}
		index := strings.Index(withGlyph, newCommitsMark)
		if index < 0 || strings.Contains(withoutGlyph, newCommitsMark) {
			test.Fatalf("selected=%v: glyph in %q, but not in %q", selected, withGlyph, withoutGlyph)
		}
		if withGlyph[:index]+" "+withGlyph[index+1:] != withoutGlyph {
			test.Errorf("selected=%v: %q differs from %q by more than the glyph", selected, withGlyph, withoutGlyph)
		}
	}
}
