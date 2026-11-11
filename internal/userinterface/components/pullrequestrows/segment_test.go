package pullrequestrows

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// twoSegments is a cell in two styles, as the review column will be: a glyph
// and a note beside it.
func twoSegments(activeTheme theme.Theme) cell {
	return cell{
		{text: "✓", style: activeTheme.PullRequestList.Approved},
		{text: " newer", style: activeTheme.PullRequestList.Age},
	}
}

func segmentTexts(content cell) []string {
	texts := make([]string, len(content))
	for i, part := range content {
		texts[i] = part.text
	}
	return texts
}

func TestCellOfTwoSegmentsPads(test *testing.T) {
	activeTheme := theme.Default()
	tests := []struct {
		name      string
		pad       func(cell, int) cell
		wantPlain string
		wantTexts []string
	}{
		{"right", cell.padRight, "✓ newer   ", []string{"✓", " newer   "}},
		{"left", cell.padLeft, "   ✓ newer", []string{"   ✓", " newer"}},
	}
	for _, testCase := range tests {
		got := testCase.pad(twoSegments(activeTheme), 10)
		if got.plain() != testCase.wantPlain || ansi.StringWidth(got.plain()) != 10 {
			test.Errorf("%s: plain %q, want %q", testCase.name, got.plain(), testCase.wantPlain)
		}
		if diff := cmp.Diff(testCase.wantTexts, segmentTexts(got)); diff != "" {
			test.Errorf("%s: segments (-want +got):\n%s", testCase.name, diff)
		}
	}
	if got := twoSegments(activeTheme).padRight(3); got.plain() != "✓ newer" {
		test.Errorf("a cell wider than the column was changed: %q", got.plain())
	}
}

func TestCellOfTwoSegmentsTruncates(test *testing.T) {
	activeTheme := theme.Default()
	tests := []struct {
		width     int
		wantTexts []string
	}{
		{20, []string{"✓", " newer"}}, // fits: untouched
		{7, []string{"✓", " newer"}},  // fits exactly
		{5, []string{"✓", " ne…"}},    // the cut falls in the second segment
		{2, []string{"✓", "…"}},       // the cut falls at the boundary
		{1, []string{"…"}},            // only the ellipsis fits
		{0, []string{}},               // nothing fits
	}
	for _, testCase := range tests {
		got := twoSegments(activeTheme).truncate(testCase.width)
		if width := ansi.StringWidth(got.plain()); width > testCase.width {
			test.Errorf("width %d: truncated to %d columns: %q", testCase.width, width, got.plain())
		}
		if diff := cmp.Diff(testCase.wantTexts, segmentTexts(got)); diff != "" {
			test.Errorf("width %d: segments (-want +got):\n%s", testCase.width, diff)
		}
	}
	// Each part keeps its own style when cut.
	got := twoSegments(activeTheme).truncate(5)
	want := activeTheme.PullRequestList.Approved.Render("✓") + activeTheme.PullRequestList.Age.Render(" ne…")
	if got.paint() != want {
		test.Errorf("cut cell painted %q, want %q", got.paint(), want)
	}
}

func TestCellOfTwoSegmentsIsPaintedPerPartAndAsAWhole(test *testing.T) {
	activeTheme := theme.Default()
	styles := activeTheme.PullRequestList
	content := twoSegments(activeTheme).padRight(10)

	want := styles.Approved.Render("✓") + styles.Age.Render(" newer   ")
	if got := content.paint(); got != want {
		test.Errorf("painted %q, want %q", got, want)
	}
	if got, want := content.paintAll(styles.Selected), styles.Selected.Render("✓ newer   "); got != want {
		test.Errorf("painted as a whole %q, want %q", got, want)
	}
	if got := ansi.Strip(content.paint()); got != "✓ newer   " {
		test.Errorf("stripped paint %q", got)
	}
}

// TestRowMadeOfTwoSegmentCells puts a two-segment cell through the layout and
// the paint step of a row, selected and not.
func TestRowMadeOfTwoSegmentCells(test *testing.T) {
	activeTheme := theme.Default()
	styles := activeTheme.PullRequestList
	pullRequests := []github.PullRequest{{Number: 7, Title: "Title", Author: "alice"}}
	table := New(activeTheme, time.Now, Options{})
	layout := table.Layout(pullRequests, 60)

	row := layout.row(rowCells{
		number:   styled("#7", styles.Number),
		title:    styled("Title", activeTheme.Normal),
		author:   styled("alice", styles.Author),
		age:      styled("2d", styles.Age),
		checks:   styled("", styles.Comments),
		review:   twoSegments(activeTheme).truncate(layout.review),
		comments: styled("3", styles.Comments),
	})
	// The review column is two columns wide: the glyph, then the ellipsis.
	if plain := row.plain(); ansi.StringWidth(plain) != 60 || !strings.Contains(plain, "✓…") {
		test.Errorf("row %q is %d columns wide, want %d with the cut review cell", plain, ansi.StringWidth(plain), 60)
	}
	if painted := row.paint(); !strings.Contains(painted, styles.Approved.Render("✓")+styles.Age.Render("…")) {
		test.Errorf("row %q lacks the review parts in their own styles", painted)
	}
	if painted := row.paintAll(styles.Selected); painted != styles.Selected.Render(row.plain()) || strings.Contains(painted, styles.Approved.Render("✓")) {
		test.Errorf("selected row %q is not painted in the selected style alone", painted)
	}
}

func TestSelectedRowIsPaintedInTheSelectedStyleAlone(test *testing.T) {
	activeTheme := theme.Default()
	styles := activeTheme.PullRequestList
	pullRequests := []github.PullRequest{{
		Number: 7, Title: "Second step", Author: "alice", Draft: true,
		Checks: github.ChecksPassing, ReviewDecision: github.ReviewDecisionApproved,
	}}
	table := New(activeTheme, time.Now, Options{})
	layout := table.Layout(pullRequests, 60-MarkerColumns)

	unselected := ansi.Strip(table.Row(layout, pullRequests[0], false, false, 60))
	selected := table.Row(layout, pullRequests[0], true, false, 60)
	if want := styles.Selected.Render(pad("> "+unselected[len(noMarker):], 60)); selected != want {
		test.Errorf("selected row %q, want %q", selected, want)
	}
	for _, own := range []lipgloss.Style{styles.Draft, styles.Approved, styles.ChecksPassing, styles.Author} {
		if strings.Contains(selected, own.Render("alice")) || strings.Contains(selected, own.Render("[draft] ")) {
			test.Errorf("selected row %q keeps a segment's own style", selected)
		}
	}
}
