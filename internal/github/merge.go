package github

import "slices"

// MergeMethod is how a pull request is merged.
type MergeMethod string

// The merge methods GitHub offers, in the order strig prefers them.
const (
	MergeMethodMerge  MergeMethod = "MERGE"
	MergeMethodSquash MergeMethod = "SQUASH"
	MergeMethodRebase MergeMethod = "REBASE"
)

// MergeMethods lists every merge method in preference order.
var MergeMethods = []MergeMethod{MergeMethodMerge, MergeMethodSquash, MergeMethodRebase}

// MergeStatus is the state a merge depends on.
type MergeStatus struct {
	// PullRequestID is the pull request's node ID.
	PullRequestID string
	// HeadSHA is the pull request's current head commit.
	HeadSHA string
	// BaseRef is the branch the pull request merges into.
	BaseRef string
	// Allowed lists the merge methods the repository allows, in the order of
	// MergeMethods.
	Allowed []MergeMethod
	// DeleteBranchOnMerge is the repository's "automatically delete head
	// branches" setting.
	DeleteBranchOnMerge bool
}

// Allows reports whether the repository allows method.
func (s MergeStatus) Allows(method MergeMethod) bool { return slices.Contains(s.Allowed, method) }

// MergeInput describes a merge.
type MergeInput struct {
	Repo          Repo
	Number        int
	PullRequestID string
	Method        MergeMethod
	// ExpectedHeadOID is the head commit the caller reviewed; GitHub refuses
	// the merge if the head has moved.
	ExpectedHeadOID string
}
