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

// pad right-pads s with spaces to width w (display columns).
func pad(text string, width int) string {
	if count := width - ansi.StringWidth(text); count > 0 {
		return text + strings.Repeat(" ", count)
	}
	return text
}

// padLeft left-pads s with spaces to width w (display columns).
func padLeft(text string, width int) string {
	if count := width - ansi.StringWidth(text); count > 0 {
		return strings.Repeat(" ", count) + text
	}
	return text
}

// relativeAge formats how long ago t was: "now", "5m", "3h", "2d", "4mo", "2y".
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
