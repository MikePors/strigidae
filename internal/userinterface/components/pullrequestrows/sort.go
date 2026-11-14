package pullrequestrows

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// SortKey is what a local sort orders pull requests by. The empty key is
// GitHub's order, the rows as they were loaded.
type SortKey string

// The sort keys, as `:sort` takes them.
const (
	SortDefault  SortKey = ""
	SortNumber   SortKey = "number"
	SortTitle    SortKey = "title"
	SortAuthor   SortKey = "author"
	SortCreated  SortKey = "created"
	SortUpdated  SortKey = "updated"
	SortReview   SortKey = "review"
	SortChecks   SortKey = "checks"
	SortComments SortKey = "comments"
	SortDraft    SortKey = "draft"
)

// sortKeys lists the keys in the order the help and errors name them.
var sortKeys = []SortKey{SortNumber, SortTitle, SortAuthor, SortCreated, SortUpdated, SortReview, SortChecks, SortComments, SortDraft}

// defaultWord is the `:sort` argument that goes back to GitHub's order.
const defaultWord = "default"

// The directions `:sort` takes after the key.
const (
	ascendingWord  = "asc"
	descendingWord = "desc"
)

// SortArguments is the usage of `:sort` after its name, for the help.
const SortArguments = "<key> [asc|desc]"

// SortDescription describes `:sort` for the help and the key reference.
const SortDescription = "sort the tab's loaded rows in strig: by number, title, author, created, updated, review, checks, comments or draft, ascending unless `desc` follows; `:sort default` goes back to GitHub's order"

// Sort is a local order of pull requests: a key and a direction. The zero
// value keeps GitHub's order.
type Sort struct {
	Key        SortKey
	Descending bool
}

// SortCompletions lists the whole arguments Tab offers after `:sort `.
func SortCompletions() []string {
	words := make([]string, 0, len(sortKeys)+1)
	for _, key := range sortKeys {
		words = append(words, string(key))
	}
	return append(words, defaultWord)
}

// ParseSort reads the arguments of `:sort`: a key and an optional direction,
// or `default`. Its errors are meant for the status bar.
func ParseSort(arguments string) (Sort, error) {
	words := strings.Fields(arguments)
	if len(words) == 0 {
		return Sort{}, errors.New(":sort needs a key: number, title, author, created, updated, review, checks, comments, draft or default")
	}
	if words[0] == defaultWord {
		if len(words) > 1 {
			return Sort{}, usageError()
		}
		return Sort{}, nil
	}
	key := SortKey(words[0])
	if !slices.Contains(sortKeys, key) {
		return Sort{}, fmt.Errorf("unknown sort key: %s", terminaltext.Clean(words[0]))
	}
	if len(words) == 1 {
		return Sort{Key: key}, nil
	}
	if len(words) > 2 || (words[1] != ascendingWord && words[1] != descendingWord) {
		return Sort{}, usageError()
	}
	return Sort{Key: key, Descending: words[1] == descendingWord}, nil
}

func usageError() error { return errors.New("usage: :sort " + SortArguments) }

// Label is how the list shows the sort, such as "↑ created"; "" for GitHub's
// order.
func (order Sort) Label() string {
	if order.Key == SortDefault {
		return ""
	}
	arrow := "↑"
	if order.Descending {
		arrow = "↓"
	}
	return arrow + " " + string(order.Key)
}

// Apply returns the pull requests in this order, leaving pullRequests as it
// was. Equal rows are ordered by number, then repository, so a refresh never
// moves them; descending is the exact reverse of ascending. GitHub's order
// returns them unchanged.
func (order Sort) Apply(pullRequests []github.PullRequest) []github.PullRequest {
	if order.Key == SortDefault {
		return pullRequests
	}
	sorted := slices.Clone(pullRequests)
	slices.SortStableFunc(sorted, func(left, right github.PullRequest) int {
		result := cmp.Or(
			order.compareKey(left, right),
			cmp.Compare(left.Number, right.Number),
			cmp.Compare(left.Repository.String(), right.Repository.String()),
		)
		if order.Descending {
			return -result
		}
		return result
	})
	return sorted
}

// compareKey compares two pull requests by the key alone, ascending.
func (order Sort) compareKey(left, right github.PullRequest) int {
	switch order.Key {
	case SortTitle:
		return compareText(left.Title, right.Title)
	case SortAuthor:
		return compareText(left.Author, right.Author)
	case SortCreated:
		return left.CreatedAt.Compare(right.CreatedAt)
	case SortUpdated:
		return left.UpdatedAt.Compare(right.UpdatedAt)
	case SortReview:
		return cmp.Compare(reviewRank(left.ReviewDecision), reviewRank(right.ReviewDecision))
	case SortChecks:
		return cmp.Compare(checksRank(left.Checks), checksRank(right.Checks))
	case SortComments:
		return cmp.Compare(left.CommentCount, right.CommentCount)
	case SortDraft:
		return cmp.Compare(draftRank(left.Draft), draftRank(right.Draft))
	default: // SortNumber: the tiebreaker does it
		return 0
	}
}

// compareText compares the text as drawn, ignoring case.
func compareText(left, right string) int {
	return cmp.Compare(strings.ToLower(terminaltext.Clean(left)), strings.ToLower(terminaltext.Clean(right)))
}

// reviewRank puts the furthest along first: approved, changes requested,
// review required, then no decision.
func reviewRank(decision github.ReviewDecision) int {
	switch decision {
	case github.ReviewDecisionApproved:
		return 0
	case github.ReviewDecisionChangesRequested:
		return 1
	case github.ReviewDecisionReviewRequired:
		return 2
	default:
		return 3
	}
}

// checksRank puts failing checks first, as `:checks` does, then running,
// passing, and none.
func checksRank(state github.CheckState) int {
	switch state {
	case github.ChecksFailing:
		return 0
	case github.ChecksRunning:
		return 1
	case github.ChecksPassing:
		return 2
	default:
		return 3
	}
}

// draftRank puts pull requests that are ready before drafts.
func draftRank(draft bool) int {
	if draft {
		return 1
	}
	return 0
}
