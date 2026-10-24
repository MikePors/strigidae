package workspace

import (
	"strings"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// reviewerMark is what the header shows after a reviewer's name.
type reviewerMark int

const (
	markRequested reviewerMark = iota // requested (or re-requested), not yet reviewed
	markApproved
	markChangesRequested
	markCommented
)

// reviewerEntry is one reviewer in the header.
type reviewerEntry struct {
	name string // raw, as GitHub gave it; "team x" for a team
	mark reviewerMark
}

// reviewerEntries lists everyone with a pending request, then everyone whose
// submitted reviews give a mark. As on github.com, the mark comes from a
// reviewer's latest opinionated review (approved or changes requested), and a
// later comment-only review does not replace it. A dismissed review is the
// latest opinionated one when it comes last: it counts as no opinion, with no
// fallback to an earlier one. A reviewer with no opinion but comment reviews
// shows as commented; one with neither is left out. The viewer's pending draft
// counts as no review, and currently the pull request's author is left out.
// Someone with a pending request shows as requested, whatever they submitted.
func reviewerEntries(detail *github.PullRequestDetail) []reviewerEntry {
	var out []reviewerEntry
	requested := map[string]bool{}
	for _, request := range detail.ReviewRequests {
		name := request.Name
		if request.IsTeam {
			name = "team " + name
		}
		if requested[name] {
			continue
		}
		requested[name] = true
		out = append(out, reviewerEntry{name: name, mark: markRequested})
	}
	type opinion struct {
		mark      reviewerMark
		opinioned bool
		commented bool
	}
	opinions := map[string]*opinion{}
	var order []string
	for _, review := range detail.Reviews { // oldest first, so later ones win
		if review.SubmittedAt.IsZero() || review.Author == detail.Author {
			continue
		}
		current := opinions[review.Author]
		if current == nil {
			current = &opinion{}
			opinions[review.Author] = current
			order = append(order, review.Author)
		}
		switch review.State {
		case github.ReviewApproved:
			current.mark, current.opinioned = markApproved, true
		case github.ReviewChangesRequested:
			current.mark, current.opinioned = markChangesRequested, true
		case github.ReviewDismissed:
			current.opinioned = false
		case github.ReviewCommented:
			current.commented = true
		case github.ReviewPending:
		}
	}
	for _, author := range order {
		current := opinions[author]
		switch {
		case requested[author]:
		case current.opinioned:
			out = append(out, reviewerEntry{name: author, mark: current.mark})
		case current.commented:
			out = append(out, reviewerEntry{name: author, mark: markCommented})
		}
	}
	return out
}

// reviewersText renders "reviewers: name ✓, …" with each mark in the pull
// request list's review colours; the comment mark is muted.
func reviewersText(activeTheme theme.Theme, entries []reviewerEntry) string {
	styles := activeTheme.PullRequestList
	parts := make([]string, len(entries))
	for i, entry := range entries {
		glyph, style := "?", styles.ReviewRequired
		switch entry.mark {
		case markApproved:
			glyph, style = "✓", styles.Approved
		case markChangesRequested:
			glyph, style = "✗", styles.ChangesRequested
		case markCommented:
			glyph, style = "•", activeTheme.Muted
		case markRequested:
		}
		parts[i] = activeTheme.Muted.Render(terminaltext.Clean(entry.name)) + " " + style.Render(glyph)
	}
	return activeTheme.Muted.Render("reviewers: ") + strings.Join(parts, activeTheme.Muted.Render(", "))
}
