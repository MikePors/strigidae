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

func TestConflictsCell(test *testing.T) {
	activeTheme := theme.Default()
	style := activeTheme.PullRequestList.Conflicts
	table := New(activeTheme, time.Now, Options{})
	tests := []struct {
		name  string
		state github.MergeState
		width int
		want  string
	}{
		{"conflicts, compact", github.MergeStateConflicts, conflictsCompactColumns, "!"},
		{"conflicts, wide", github.MergeStateConflicts, conflictsWideColumns, "conflicts"},
		{"behind", github.MergeStateBehind, conflictsWideColumns, ""},
		{"blocked", github.MergeStateBlocked, conflictsCompactColumns, ""},
		{"not known", github.MergeStateOther, conflictsWideColumns, ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got := table.conflictsCell(github.PullRequest{MergeState: testCase.state}, testCase.width)
			if got.plain() != testCase.want {
				test.Errorf("text = %q, want %q", got.plain(), testCase.want)
			}
			if testCase.want == "" {
				return
			}
			// The theme's own style, and not merely some text: it must add colour.
			if want := style.Render(testCase.want); got.paint() != want || want == testCase.want {
				test.Errorf("painted %q, want %q in the conflicts style", got.paint(), want)
			}
		})
	}
}

func conflictsTestPullRequests() []github.PullRequest {
	return []github.PullRequest{
		{Number: 1, Title: "clean", Author: "alice"},
		{Number: 2, Title: "conflicting", Author: "alice", MergeState: github.MergeStateConflicts},
	}
}

// titleColumns is how wide the title is in a table drawn width columns wide.
func titleColumns(table Table, pullRequests []github.PullRequest, width int) int {
	return table.Layout(pullRequests, width).title
}

func TestConflictsColumnSpellsItselfOutWhileTheTitleKeepsFortyColumns(test *testing.T) {
	marked := Options{Mark: func(github.PullRequest) string { return "waiting on someone" }}
	tests := []struct {
		name    string
		options Options
	}{
		{"plain", Options{}},
		{"branches", Options{Branches: true}},
		{"repository", Options{Repository: true}},
		{"marked", marked},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			table := New(theme.Default(), time.Now, testCase.options)
			pullRequests := conflictsTestPullRequests()
			// The width at which the title is exactly comfortable with the word.
			roomy := table.Layout(pullRequests, 500).fixed() + comfortableTitleColumns
			for _, step := range []struct {
				width       int
				wantWord    bool
				wantHeading string
				wantRow     string
			}{
				{roomy, true, "CONFLICTS", "conflicts"},
				{roomy - 1, false, "CF", "!"},
			} {
				layout := table.Layout(pullRequests, step.width)
				if got := layout.conflicts == conflictsWideColumns; got != step.wantWord {
					test.Fatalf("width %d: word shown = %v, want %v (layout %+v)", step.width, got, step.wantWord, layout)
				}
				if step.wantWord && titleColumns(table, pullRequests, step.width) < comfortableTitleColumns {
					test.Errorf("width %d: the word leaves the title %d columns", step.width, layout.title)
				}
				header := ansi.Strip(table.Header(layout, step.width))
				row := ansi.Strip(table.Row(layout, pullRequests[1], false, false, step.width))
				if !strings.Contains(header, step.wantHeading) || !strings.Contains(row, step.wantRow) {
					test.Fatalf("width %d: header %q, row %q; want %q and %q", step.width, header, row, step.wantHeading, step.wantRow)
				}
				// Heading and cell start in the same column, whichever the form.
				headingIndex, cellIndex := strings.Index(header, step.wantHeading), strings.LastIndex(row, step.wantRow) // both found above
				headingColumn, cellColumn := ansi.StringWidth(header[:headingIndex]), ansi.StringWidth(row[:cellIndex])
				if headingColumn != cellColumn {
					test.Errorf("width %d: heading at column %d, cell at column %d\n%s\n%s", step.width, headingColumn, cellColumn, header, row)
				}
				if clean := ansi.Strip(table.Row(layout, pullRequests[0], false, false, step.width)); strings.Contains(clean, step.wantRow) {
					test.Errorf("width %d: clean row %q shows a conflict", step.width, clean)
				}
			}
		})
	}
}

func TestMarkDoesNotCostTheTitleItsComfort(test *testing.T) {
	// The mark stays while the title keeps twelve columns, so the conflicts
	// word must not be what takes the title below forty.
	table := New(theme.Default(), time.Now, Options{Mark: func(github.PullRequest) string { return "waiting on someone" }})
	pullRequests := conflictsTestPullRequests()
	marks := 0
	for width := 200; width > 30; width-- {
		layout := table.Layout(pullRequests, width)
		if layout.conflicts == conflictsWideColumns && layout.title < comfortableTitleColumns {
			test.Errorf("width %d: CONFLICTS shown with a title of %d columns (layout %+v)", width, layout.title, layout)
		}
		if layout.mark > 0 {
			marks++
		}
	}
	if marks == 0 {
		test.Error("the mark column never showed, so nothing was checked")
	}
}

func TestConflictsColumnGoesBeforeChecksAndReview(test *testing.T) {
	table := New(theme.Default(), time.Now, Options{})
	pullRequests := conflictsTestPullRequests()
	sawCompact, sawGone := false, false
	for width := 200; width > 10; width-- {
		layout := table.Layout(pullRequests, width)
		sawCompact = sawCompact || layout.conflicts == conflictsCompactColumns
		sawGone = sawGone || layout.conflicts == 0
		if layout.conflicts == 0 && layout.comments != 0 {
			test.Errorf("width %d: conflicts dropped before comments", width)
		}
		if (layout.checks == 0 || layout.review == 0) && layout.conflicts != 0 {
			test.Errorf("width %d: checks or review dropped while the conflicts column stayed (layout %+v)", width, layout)
		}
	}
	if !sawCompact || !sawGone {
		test.Errorf("widths 200 to 11 never showed the compact (%v) and the dropped (%v) column", sawCompact, sawGone)
	}
	// And on the way down there is a width where conflicts is gone and both
	// of the columns that outrank it remain.
	for width := 200; width > 10; width-- {
		if layout := table.Layout(pullRequests, width); layout.conflicts == 0 {
			if layout.checks == 0 || layout.review == 0 {
				test.Errorf("width %d: the first width without conflicts has checks %d review %d", width, layout.checks, layout.review)
			}
			break
		}
	}
}
