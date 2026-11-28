// Package pullrequestrows draws pull request table rows: the column layout,
// header and one row, shared by every screen that lists pull requests.
package pullrequestrows

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
)

// pad right-pads text with spaces to width columns (display columns).
func pad(text string, width int) string {
	if count := width - ansi.StringWidth(text); count > 0 {
		return text + strings.Repeat(" ", count)
	}
	return text
}

// relativeAge formats how long ago timestamp was: "now", "5m", "3h", "2d", "4mo", "2y".
func relativeAge(now, timestamp time.Time) string {
	duration := now.Sub(timestamp)
	switch {
	case timestamp.IsZero():
		return ""
	case duration < time.Minute:
		return "now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration/time.Minute))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration/time.Hour))
	case duration < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(duration/(24*time.Hour)))
	case duration < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(duration/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy", int(duration/(365*24*time.Hour)))
	}
}

func commentText(count int) string {
	if count > 99 {
		return "99+"
	}
	return fmt.Sprint(count)
}

// newCommitsMark is the glyph for commits pushed since the viewer's latest
// review. The selected row is painted in one style, so the shape alone tells it
// apart there.
const newCommitsMark = "+"

// reviewCell is the review column: the decision glyph, then the glyph for new
// commits since the viewer's review. Each takes one column, padded as plain
// text before it is styled.
func (table Table) reviewCell(pullRequest github.PullRequest) cell {
	decisionMark, decisionStyle := table.reviewMark(pullRequest.ReviewDecision)
	content := cell{{text: pad(decisionMark, 1), style: decisionStyle}}
	if pullRequest.ViewerReviewedSHA != "" && pullRequest.ViewerReviewedSHA != pullRequest.HeadSHA {
		content = append(content, segment{text: newCommitsMark, style: table.theme.PullRequestList.NewCommits})
	}
	return content
}

// reviewMark returns the one-column glyph and style for a review decision.
func (table Table) reviewMark(decision github.ReviewDecision) (string, lipgloss.Style) {
	styles := table.theme.PullRequestList
	switch decision {
	case github.ReviewDecisionApproved:
		return "✓", styles.Approved
	case github.ReviewDecisionChangesRequested:
		return "✗", styles.ChangesRequested
	case github.ReviewDecisionReviewRequired:
		return "?", styles.ReviewRequired
	default:
		return "", styles.Comments
	}
}

// conflictsMark and conflictsWord say that a pull request's branch conflicts
// with its base: the glyph for the compact column, the word for the wide one.
const (
	conflictsMark = "!"
	conflictsWord = "conflicts"
)

// conflictsHeading is the heading of the conflicts column in its width.
func (layout Layout) conflictsHeading() string {
	if layout.conflicts >= conflictsWideColumns {
		return "CONFLICTS"
	}
	return "CF"
}

// conflictsCell is the conflicts column, width columns wide: the word or the
// glyph for a pull request that cannot merge for conflicts, blank otherwise.
// Only the shape shows on the selected row, which is painted in one style.
func (table Table) conflictsCell(pullRequest github.PullRequest, width int) cell {
	text := ""
	switch {
	case pullRequest.MergeState != github.MergeStateConflicts:
	case width >= conflictsWideColumns:
		text = conflictsWord
	default:
		text = conflictsMark
	}
	return styled(text, table.theme.PullRequestList.Conflicts)
}

// checksMark returns the one-column glyph and style for the head commit's
// check state; no checks gives a blank.
func (table Table) checksMark(state github.CheckState) (string, lipgloss.Style) {
	styles := table.theme.PullRequestList
	switch state {
	case github.ChecksPassing:
		return "✓", styles.ChecksPassing
	case github.ChecksFailing:
		return "✗", styles.ChecksFailing
	case github.ChecksRunning:
		return "●", styles.ChecksRunning
	default:
		return "", styles.Comments
	}
}
