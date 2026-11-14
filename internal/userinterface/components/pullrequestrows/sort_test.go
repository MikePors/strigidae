package pullrequestrows

import (
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestParseSort(test *testing.T) {
	for _, testCase := range []struct {
		arguments string
		want      Sort
		err       string
	}{
		{"created", Sort{Key: SortCreated}, ""},
		{"created asc", Sort{Key: SortCreated}, ""},
		{"  updated   desc ", Sort{Key: SortUpdated, Descending: true}, ""},
		{"default", Sort{}, ""},
		{"", Sort{}, ":sort needs a key: number, title, author, created, updated, review, checks, comments, draft or default"},
		{"age", Sort{}, "unknown sort key: age"},
		{"a\x1b[31mb", Sort{}, "unknown sort key: a [31mb"},
		{"title up", Sort{}, "usage: :sort <key> [asc|desc]"},
		{"title asc desc", Sort{}, "usage: :sort <key> [asc|desc]"},
		{"default desc", Sort{}, "usage: :sort <key> [asc|desc]"},
		{"Title", Sort{}, "unknown sort key: Title"},
	} {
		test.Run(testCase.arguments, func(test *testing.T) {
			got, err := ParseSort(testCase.arguments)
			if testCase.err != "" {
				if err == nil || err.Error() != testCase.err {
					test.Fatalf("error = %v, want %q", err, testCase.err)
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			if got != testCase.want {
				test.Errorf("got %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestSortLabel(test *testing.T) {
	for _, testCase := range []struct {
		order Sort
		want  string
	}{
		{Sort{}, ""},
		{Sort{Key: SortCreated}, "↑ created"},
		{Sort{Key: SortComments, Descending: true}, "↓ comments"},
	} {
		if got := testCase.order.Label(); got != testCase.want {
			test.Errorf("%+v: got %q, want %q", testCase.order, got, testCase.want)
		}
	}
}

func TestSortCompletions(test *testing.T) {
	want := []string{"number", "title", "author", "created", "updated", "review", "checks", "comments", "draft", "default"}
	if diff := cmp.Diff(want, SortCompletions()); diff != "" {
		test.Errorf("(-want +got):\n%s", diff)
	}
}

// sortFixture holds one pull request per value of every key, numbered so the
// number never agrees with the key's order by accident.
func sortFixture() []github.PullRequest {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	return []github.PullRequest{
		{Repository: repository, Number: 3, Title: "beta", Author: "Carol", CreatedAt: base.Add(2 * time.Hour), UpdatedAt: base.Add(1 * time.Hour), ReviewDecision: github.ReviewDecisionReviewRequired, Checks: github.ChecksPassing, CommentCount: 5, Draft: true},
		{Repository: repository, Number: 1, Title: "Gamma", Author: "alice", CreatedAt: base.Add(3 * time.Hour), UpdatedAt: base.Add(4 * time.Hour), ReviewDecision: github.ReviewDecisionNone, Checks: github.ChecksFailing, CommentCount: 0},
		{Repository: repository, Number: 4, Title: "alpha", Author: "bob", CreatedAt: base.Add(1 * time.Hour), UpdatedAt: base.Add(3 * time.Hour), ReviewDecision: github.ReviewDecisionApproved, Checks: github.ChecksNone, CommentCount: 2},
		{Repository: repository, Number: 2, Title: "Delta", Author: "dave", CreatedAt: base.Add(4 * time.Hour), UpdatedAt: base.Add(2 * time.Hour), ReviewDecision: github.ReviewDecisionChangesRequested, Checks: github.ChecksRunning, CommentCount: 9, Draft: true},
	}
}

func numbers(pullRequests []github.PullRequest) []int {
	out := make([]int, len(pullRequests))
	for i, pullRequest := range pullRequests {
		out[i] = pullRequest.Number
	}
	return out
}

func TestSortApplyEveryKey(test *testing.T) {
	for _, testCase := range []struct {
		key  SortKey
		want []int // ascending; descending is the reverse
	}{
		{SortNumber, []int{1, 2, 3, 4}},
		{SortTitle, []int{4, 3, 2, 1}},    // alpha, beta, Delta, Gamma: case ignored
		{SortAuthor, []int{1, 4, 3, 2}},   // alice, bob, Carol, dave
		{SortCreated, []int{4, 3, 1, 2}},  // oldest first
		{SortUpdated, []int{3, 2, 4, 1}},  // oldest first
		{SortReview, []int{4, 2, 3, 1}},   // approved, changes requested, review required, none
		{SortChecks, []int{1, 2, 3, 4}},   // failing, running, passing, none
		{SortComments, []int{1, 4, 3, 2}}, // fewest first
		{SortDraft, []int{1, 4, 2, 3}},    // ready first; equal rows by number
		{SortDefault, []int{3, 1, 4, 2}},  // as loaded
	} {
		test.Run(string(testCase.key), func(test *testing.T) {
			ascending := Sort{Key: testCase.key}.Apply(sortFixture())
			if diff := cmp.Diff(testCase.want, numbers(ascending)); diff != "" {
				test.Errorf("ascending (-want +got):\n%s", diff)
			}
			if testCase.key == SortDefault {
				return
			}
			descending := Sort{Key: testCase.key, Descending: true}.Apply(sortFixture())
			reversed := slices.Clone(testCase.want)
			slices.Reverse(reversed)
			if diff := cmp.Diff(reversed, numbers(descending)); diff != "" {
				test.Errorf("descending (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSortTiebreaker(test *testing.T) {
	repository := func(name string) github.Repository { return github.Repository{Owner: "acme", Name: name} }
	same := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []github.PullRequest{
		{Repository: repository("widgets"), Number: 7, UpdatedAt: same},
		{Repository: repository("gadgets"), Number: 7, UpdatedAt: same},
		{Repository: repository("widgets"), Number: 2, UpdatedAt: same},
	}
	type row struct {
		Repository string
		Number     int
	}
	identities := func(pullRequests []github.PullRequest) []row {
		out := make([]row, len(pullRequests))
		for i, pullRequest := range pullRequests {
			out[i] = row{pullRequest.Repository.String(), pullRequest.Number}
		}
		return out
	}
	ascending := []row{{"acme/widgets", 2}, {"acme/gadgets", 7}, {"acme/widgets", 7}}
	if diff := cmp.Diff(ascending, identities(Sort{Key: SortUpdated}.Apply(rows))); diff != "" {
		test.Errorf("ascending (-want +got):\n%s", diff)
	}
	// Whatever order the rows arrive in, equal rows come out the same.
	slices.Reverse(rows)
	if diff := cmp.Diff(ascending, identities(Sort{Key: SortUpdated}.Apply(rows))); diff != "" {
		test.Errorf("reloaded in another order (-want +got):\n%s", diff)
	}
	descending := slices.Clone(ascending)
	slices.Reverse(descending)
	if diff := cmp.Diff(descending, identities(Sort{Key: SortUpdated, Descending: true}.Apply(rows))); diff != "" {
		test.Errorf("descending (-want +got):\n%s", diff)
	}
}

func TestSortApplyLeavesItsInputAlone(test *testing.T) {
	rows := sortFixture()
	Sort{Key: SortNumber}.Apply(rows)
	if diff := cmp.Diff([]int{3, 1, 4, 2}, numbers(rows)); diff != "" {
		test.Errorf("input changed (-want +got):\n%s", diff)
	}
}
