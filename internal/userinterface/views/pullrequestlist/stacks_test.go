package pullrequestlist

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
)

func stacked(number int, head, base string) github.PullRequest {
	return github.PullRequest{Number: number, HeadBranch: head, BaseBranch: base, State: github.PullRequestStateOpen}
}

func forked(number int, head, base string) github.PullRequest {
	pullRequest := stacked(number, head, base)
	pullRequest.CrossRepository = true
	return pullRequest
}

func TestStackOrder(test *testing.T) {
	closed := stacked(2, "b", "a")
	closed.State = github.PullRequestStateClosed

	for _, testCase := range []struct {
		name         string
		pullRequests []github.PullRequest
		wantNumbers  []int
		wantDepths   []int
	}{
		{
			name:         "no stacks keeps the order",
			pullRequests: []github.PullRequest{stacked(5, "e", "main"), stacked(3, "c", "main"), stacked(9, "i", "main")},
			wantNumbers:  []int{5, 3, 9},
			wantDepths:   []int{0, 0, 0},
		},
		{
			name:         "three level stack, children listed before their parents' later siblings",
			pullRequests: []github.PullRequest{stacked(10, "x", "main"), stacked(3, "c", "b"), stacked(2, "b", "a"), stacked(1, "a", "main")},
			wantNumbers:  []int{10, 1, 2, 3},
			wantDepths:   []int{0, 0, 1, 2},
		},
		{
			name:         "two siblings keep their relative order",
			pullRequests: []github.PullRequest{stacked(7, "s2", "base"), stacked(8, "other", "main"), stacked(4, "s1", "base"), stacked(6, "base", "main")},
			wantNumbers:  []int{8, 6, 7, 4},
			wantDepths:   []int{0, 0, 1, 1},
		},
		{
			name:         "missing parent leaves the child top-level",
			pullRequests: []github.PullRequest{stacked(2, "b", "a"), stacked(1, "z", "main")},
			wantNumbers:  []int{2, 1},
			wantDepths:   []int{0, 0},
		},
		{
			name:         "a fork pull request is not a parent",
			pullRequests: []github.PullRequest{forked(1, "a", "main"), stacked(2, "b", "a")},
			wantNumbers:  []int{1, 2},
			wantDepths:   []int{0, 0},
		},
		{
			name:         "a closed pull request is not a child",
			pullRequests: []github.PullRequest{stacked(1, "a", "main"), closed},
			wantNumbers:  []int{1, 2},
			wantDepths:   []int{0, 0},
		},
		{
			name:         "the first of two parents with the same head wins",
			pullRequests: []github.PullRequest{stacked(1, "a", "main"), stacked(2, "a", "main"), stacked(3, "c", "a")},
			wantNumbers:  []int{1, 3, 2},
			wantDepths:   []int{0, 1, 0},
		},
		{
			name:         "a cycle shows every row once",
			pullRequests: []github.PullRequest{stacked(1, "a", "b"), stacked(2, "b", "a"), stacked(3, "c", "main")},
			wantNumbers:  []int{1, 2, 3},
			wantDepths:   []int{0, 1, 0},
		},
		{
			name:         "a row stacked on a cycle is drawn below the cycle's first row",
			pullRequests: []github.PullRequest{stacked(3, "c", "a"), stacked(1, "a", "b"), stacked(2, "b", "a")},
			wantNumbers:  []int{1, 3, 2},
			wantDepths:   []int{0, 1, 1},
		},
		{
			name:         "a pull request whose base is its own head is top-level",
			pullRequests: []github.PullRequest{stacked(1, "a", "a")},
			wantNumbers:  []int{1},
			wantDepths:   []int{0},
		},
		{
			name:         "rows without branches do not stack",
			pullRequests: []github.PullRequest{stacked(1, "", ""), stacked(2, "", "")},
			wantNumbers:  []int{1, 2},
			wantDepths:   []int{0, 0},
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			ordered, depths := stackOrder(testCase.pullRequests, pullrequestrows.Sort{})
			gotNumbers := make([]int, len(ordered))
			for i, pullRequest := range ordered {
				gotNumbers[i] = pullRequest.Number
			}
			if diff := cmp.Diff(testCase.wantNumbers, gotNumbers); diff != "" {
				test.Errorf("order mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.wantDepths, depths); diff != "" {
				test.Errorf("depths mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStackParentDoesNotDependOnTheSort(test *testing.T) {
	// #1 and #2 share the head branch feat; #3 builds on feat. Loaded first,
	// #2 is its parent, in every sort.
	loaded := []github.PullRequest{stacked(2, "feat", "release"), stacked(1, "feat", "main"), stacked(3, "child", "feat")}
	for _, testCase := range []struct {
		order pullrequestrows.Sort
		want  []int
	}{
		{pullrequestrows.Sort{}, []int{2, 3, 1}},
		{pullrequestrows.Sort{Key: pullrequestrows.SortNumber}, []int{1, 2, 3}},
		{pullrequestrows.Sort{Key: pullrequestrows.SortNumber, Descending: true}, []int{2, 3, 1}},
	} {
		ordered, depths := stackOrder(loaded, testCase.order)
		var numbers []int
		for _, pullRequest := range ordered {
			numbers = append(numbers, pullRequest.Number)
		}
		if diff := cmp.Diff(testCase.want, numbers); diff != "" {
			test.Errorf("%+v rows (-want +got):\n%s", testCase.order, diff)
		}
		child := 0
		for i, pullRequest := range ordered {
			if pullRequest.Number == 3 {
				child = i
			}
		}
		if child == 0 || ordered[child-1].Number != 2 || depths[child] != 1 {
			test.Errorf("%+v: #3 is not directly under #2: %v, depths %v", testCase.order, numbers, depths)
		}
	}
}
