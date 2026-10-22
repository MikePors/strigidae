package prlist

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
)

// pad right-pads s with spaces to width w (display columns).
func pad(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// padLeft left-pads s with spaces to width w (display columns).
func padLeft(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// relativeAge formats how long ago t was: "now", "5m", "3h", "2d", "4mo", "2y".
func relativeAge(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case t.IsZero():
		return ""
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy", int(d/(365*24*time.Hour)))
	}
}

func commentText(n int) string {
	if n > 99 {
		return "99+"
	}
	return fmt.Sprint(n)
}

// reviewMark returns the one-column glyph and style for a review decision.
func (v *View) reviewMark(d github.ReviewDecision) (string, lipgloss.Style) {
	st := v.th.PRList
	switch d {
	case github.ReviewDecisionApproved:
		return "✓", st.Approved
	case github.ReviewDecisionChangesRequested:
		return "✗", st.ChangesRequested
	case github.ReviewDecisionReviewRequired:
		return "?", st.ReviewRequired
	default:
		return "", st.Comments
	}
}

// checksMark returns the one-column glyph and style for the head commit's
// check state; no checks gives a blank.
func (v *View) checksMark(c github.CheckState) (string, lipgloss.Style) {
	st := v.th.PRList
	switch c {
	case github.ChecksPassing:
		return "✓", st.ChecksPassing
	case github.ChecksFailing:
		return "✗", st.ChecksFailing
	case github.ChecksRunning:
		return "●", st.ChecksRunning
	default:
		return "", st.Comments
	}
}
