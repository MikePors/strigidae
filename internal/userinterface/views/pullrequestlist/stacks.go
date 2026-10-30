package pullrequestlist

import "github.com/MikePors/strigidae/internal/github"

// stackOrder arranges pullRequests, in their loaded order, so that each pull
// request is followed by the ones stacked on it, and reports how deep each row
// is in its stack (0 for a top-level row).
//
// A pull request is stacked on another when its base branch is the other's
// head branch, both are open, and the other's head branch lives in this
// repository: a fork's branch may share a name with one here. Pull requests
// whose parent is not in the list stay top-level. When two candidates share a
// head branch name the first in order is the parent. Every pull request
// appears exactly once, even when bases and heads point at each other.
func stackOrder(pullRequests []github.PullRequest) (ordered []github.PullRequest, depths []int) {
	parents := make([]int, len(pullRequests))
	children := make([][]int, len(pullRequests))
	for i := range pullRequests {
		parents[i] = stackParent(pullRequests, i)
		if parents[i] >= 0 {
			children[parents[i]] = append(children[parents[i]], i)
		}
	}

	ordered = make([]github.PullRequest, 0, len(pullRequests))
	depths = make([]int, 0, len(pullRequests))
	visited := make([]bool, len(pullRequests))
	var place func(index, depth int)
	place = func(index, depth int) {
		visited[index] = true
		ordered = append(ordered, pullRequests[index])
		depths = append(depths, depth)
		for _, child := range children[index] {
			if !visited[child] {
				place(child, depth+1)
			}
		}
	}
	// A row is top-level when it has no parent, or when its parents lead into
	// a cycle instead of a top-level row: the cycle is broken at its first row
	// in order.
	for i := range pullRequests {
		if !visited[i] && (parents[i] < 0 || !reachesTop(parents, i)) {
			place(i, 0)
		}
	}
	return ordered, depths
}

// stackParent is the index of the pull request that pullRequests[index] is
// stacked on, or -1.
func stackParent(pullRequests []github.PullRequest, index int) int {
	child := pullRequests[index]
	if child.State != github.PullRequestStateOpen || child.BaseBranch == "" {
		return -1
	}
	for i, candidate := range pullRequests {
		if i != index && candidate.State == github.PullRequestStateOpen && !candidate.CrossRepository && candidate.HeadBranch == child.BaseBranch {
			return i
		}
	}
	return -1
}

// reachesTop reports whether following parents from index ends at a row
// without one, rather than going round a cycle.
func reachesTop(parents []int, index int) bool {
	for steps := 0; steps <= len(parents); steps++ {
		if parents[index] < 0 {
			return true
		}
		index = parents[index]
	}
	return false
}
