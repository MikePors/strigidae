package api

import "fmt"

// Operation names double as keys for fixtures in tests.
const (
	opPullRequestList   = "PullRequestList"
	opPullRequestSearch = "PullRequestSearch"
	opPullRequestDetail = "PullRequestDetail"
	opReviewThreads     = "ReviewThreads"
	opReplyToThread     = "ReplyToThread"
)

const reactionSelection = `reactionGroups { content viewerHasReacted reactors { totalCount } }`

const pullRequestFields = `
fragment PullRequestFields on PullRequest {
  id number title url isDraft state createdAt updatedAt reviewDecision
  baseRefName headRefName headRefOid
  author { login }
  commentCount: comments { totalCount }
  repository { nameWithOwner }
}`

const (
	fileSelection = `path additions deletions changeType`

	issueCommentSelection = `id url body createdAt author { login } ` + reactionSelection

	reviewSelection = `id state body submittedAt author { login }`

	reviewCommentSelection = `id url state body diffHunk createdAt author { login } ` + reactionSelection
)

const listPullRequestsQuery = `
query ` + opPullRequestList + `($owner: String!, $name: String!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: $first, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
      totalCount
      pageInfo { hasNextPage endCursor }
      nodes { ...PullRequestFields }
    }
  }
}` + pullRequestFields

const searchPullRequestsQuery = `
query ` + opPullRequestSearch + `($query: String!, $first: Int!, $after: String) {
  search(query: $query, type: ISSUE, first: $first, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes { ...PullRequestFields }
  }
}` + pullRequestFields

// detailQuery builds the pull request detail query. Reading Team fields needs
// the read:org scope, so withTeams=false asks only for the type name of
// requested reviewers.
func detailQuery(withTeams bool) string {
	reviewer := `... on User { login } ... on Team { slug } ... on Bot { login } ... on Mannequin { login }`
	if !withTeams {
		reviewer = `... on User { login } ... on Bot { login } ... on Mannequin { login }`
	}
	return fmt.Sprintf(pullRequestDetailQueryFmt, reviewer)
}

const pullRequestDetailQueryFmt = `
query ` + opPullRequestDetail + `($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      ...PullRequestFields
      body additions deletions
      labels(first: 50) { nodes { name color } }
      reviewRequests(first: 50) {
        nodes { requestedReviewer { __typename %s } }
      }
      files(first: 100) { pageInfo { hasNextPage endCursor } nodes { ` + fileSelection + ` } }
      comments(first: 100) { pageInfo { hasNextPage endCursor } nodes { ` + issueCommentSelection + ` } }
      reviews(first: 100) { pageInfo { hasNextPage endCursor } nodes { ` + reviewSelection + ` } }
    }
  }
}` + pullRequestFields

// moreQuery builds the query that continues one of the detail connections
// (files, comments, reviews). The connection is aliased to "conn".
func moreQuery(op, field, selection string) string {
	return fmt.Sprintf(`
query %s($owner: String!, $name: String!, $number: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      conn: %s(first: 100, after: $after) { pageInfo { hasNextPage endCursor } nodes { %s } }
    }
  }
}`, op, field, selection)
}

var (
	moreFilesQuery    = moreQuery("PullRequestFiles", "files", fileSelection)
	moreCommentsQuery = moreQuery("PullRequestComments", "comments", issueCommentSelection)
	moreReviewsQuery  = moreQuery("PullRequestReviews", "reviews", reviewSelection)
)

const reviewThreadsQuery = `
query ` + opReviewThreads + `($owner: String!, $name: String!, $number: Int!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: $first, after: $after) {
        totalCount
        pageInfo { hasNextPage endCursor }
        nodes {
          id subjectType isResolved isOutdated path line originalLine startLine originalStartLine diffSide startDiffSide
          resolvedBy { login }
          comments(first: 100) { nodes { ` + reviewCommentSelection + ` } }
        }
      }
    }
  }
}`

const replyToThreadMutation = `
mutation ` + opReplyToThread + `($threadId: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $threadId, body: $body}) {
    comment { ` + reviewCommentSelection + ` }
  }
}`
