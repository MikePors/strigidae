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
// latest submitted review is on record. Currently the pull request's author is
// left out, and dismissed reviews and the viewer's pending draft count as no
// review. Someone asked to review again shows as requested.
func reviewerEntries(detail *github.PullRequestDetail) []reviewerEntry {
	var out []reviewerEntry
	index := map[string]int{}
	for _, request := range detail.ReviewRequests {
		name := request.Name
		if request.IsTeam {
			name = "team " + name
		}
		if _, seen := index[name]; seen {
			continue
		}
		index[name] = len(out)
		out = append(out, reviewerEntry{name: name, mark: markRequested})
	}
	requested := len(out)
	latest := map[string]reviewerMark{}
	var order []string
	for _, review := range detail.Reviews { // oldest first, so later ones win
		mark, ok := markOf(review)
		if !ok || review.Author == detail.Author {
			continue
		}
		if _, seen := latest[review.Author]; !seen {
			order = append(order, review.Author)
		}
		latest[review.Author] = mark
	}
	for _, author := range order {
		if position, requestedAgain := index[author]; requestedAgain && position < requested {
			continue
		}
		out = append(out, reviewerEntry{name: author, mark: latest[author]})
	}
	return out
}

func markOf(review github.Review) (reviewerMark, bool) {
	if review.SubmittedAt.IsZero() {
		return 0, false
	}
	switch review.State {
	case github.ReviewApproved:
		return markApproved, true
	case github.ReviewChangesRequested:
		return markChangesRequested, true
	case github.ReviewCommented:
		return markCommented, true
	case github.ReviewDismissed, github.ReviewPending:
	}
	return 0, false
}

// reviewersText renders "reviewers: name✓, …" with each mark in the pull
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
