package network

import (
	"time"

	"github.com/MikePors/strigidae/internal/github"
)

// ghost is shown for authors whose account was deleted (GraphQL returns null).
const ghost = "ghost"

type graphqlActor struct{ Login string }

func (actor *graphqlActor) login() string {
	if actor == nil || actor.Login == "" {
		return ghost
	}
	return actor.Login
}

type graphqlPageInfo struct {
	HasNextPage bool
	EndCursor   string
}

func (pageInformation graphqlPageInfo) next() string {
	if !pageInformation.HasNextPage {
		return ""
	}
	return pageInformation.EndCursor
}

type graphqlConnection[Node any] struct {
	TotalCount int
	PageInfo   graphqlPageInfo
	Nodes      []Node
}

type graphqlPullRequest struct {
	ID                string
	Number            int
	Title             string
	URL               string
	IsDraft           bool
	State             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ReviewDecision    string
	BaseRefName       string
	HeadRefName       string
	HeadRefOid        string
	StatusCheckRollup *struct{ State string }
	Author            *graphqlActor
	CommentCount      struct{ TotalCount int }
	Repository        struct{ NameWithOwner string }
}

type graphqlReactionGroup struct {
	Content          string
	ViewerHasReacted bool
	Reactors         struct{ TotalCount int }
}

type graphqlIssueComment struct {
	ID             string
	URL            string
	Body           string
	CreatedAt      time.Time
	Author         *graphqlActor
	ReactionGroups []graphqlReactionGroup
}

type graphqlReviewComment struct {
	ID             string
	State          string
	URL            string
	Body           string
	DiffHunk       string
	CreatedAt      time.Time
	Author         *graphqlActor
	ReactionGroups []graphqlReactionGroup

	ViewerCanUpdate bool
	ViewerCanDelete bool
	ViewerDidAuthor bool
}

type graphqlFile struct {
	Path       string
	Additions  int
	Deletions  int
	ChangeType string
	// ViewerViewedState is VIEWED, UNVIEWED or DISMISSED.
	ViewerViewedState string
}

type graphqlReview struct {
	ID          string
	State       string
	Body        string
	SubmittedAt time.Time
	Author      *graphqlActor
}

type graphqlPullRequestDetail struct {
	graphqlPullRequest
	ViewerDidAuthor bool
	Body            string
	Additions       int
	Deletions       int
	Labels          struct {
		Nodes []struct{ Name, Color string }
	}

	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *struct {
				Typename string `json:"__typename"`
				Login    string
				Slug     string
			}
		}
	}
	Files    graphqlConnection[graphqlFile]
	Comments graphqlConnection[graphqlIssueComment]
	Reviews  graphqlConnection[graphqlReview]
}

type graphqlThread struct {
	ID                string
	SubjectType       string
	IsResolved        bool
	IsOutdated        bool
	Path              string
	Line              *int
	OriginalLine      *int
	StartLine         *int
	OriginalStartLine *int
	DiffSide          string
	StartDiffSide     string
	ResolvedBy        *graphqlActor
	Comments          struct{ Nodes []graphqlReviewComment }
}

func mapPullRequest(node graphqlPullRequest) (github.PullRequest, error) {
	repository, err := github.ParseRepository(node.Repository.NameWithOwner)
	if err != nil {
		return github.PullRequest{}, err
	}
	return github.PullRequest{
		Identifier:     node.ID,
		HeadSHA:        node.HeadRefOid,
		Repository:     repository,
		Number:         node.Number,
		Title:          node.Title,
		Author:         node.Author.login(),
		State:          github.PullRequestState(node.State),
		Draft:          node.IsDraft,
		ReviewDecision: github.ReviewDecision(node.ReviewDecision),
		Checks:         mapChecks(node),
		CommentCount:   node.CommentCount.TotalCount,
		CreatedAt:      node.CreatedAt,
		UpdatedAt:      node.UpdatedAt,
		URL:            node.URL,
		HeadBranch:     node.HeadRefName,
		BaseBranch:     node.BaseRefName,
	}, nil
}

// mapChecks reads the head commit's rollup; null means it has no checks.
func mapChecks(node graphqlPullRequest) github.CheckState {
	if node.StatusCheckRollup == nil {
		return github.ChecksNone
	}
	return github.CheckStateFromRollup(node.StatusCheckRollup.State)
}

func mapPullRequests(nodes []graphqlPullRequest) ([]github.PullRequest, error) {
	out := make([]github.PullRequest, 0, len(nodes))
	for _, node := range nodes {
		// Search results can contain nulls for inaccessible items, which decode
		// to zero values.
		if node.Number == 0 {
			continue
		}
		pullRequest, err := mapPullRequest(node)
		if err != nil {
			return nil, err
		}
		out = append(out, pullRequest)
	}
	return out, nil
}

func mapReactions(groups []graphqlReactionGroup) []github.ReactionSummary {
	var out []github.ReactionSummary
	for _, group := range groups {
		if group.Reactors.TotalCount == 0 {
			continue
		}
		out = append(out, github.ReactionSummary{
			Content:       github.ReactionContent(group.Content),
			Count:         group.Reactors.TotalCount,
			ViewerReacted: group.ViewerHasReacted,
		})
	}
	return out
}

func mapIssueComment(node graphqlIssueComment) github.IssueComment {
	return github.IssueComment{
		Identifier: node.ID,
		Author:     node.Author.login(),
		Body:       node.Body,
		CreatedAt:  node.CreatedAt,
		URL:        node.URL,
		Reactions:  mapReactions(node.ReactionGroups),
	}
}

func mapReviewComment(node graphqlReviewComment) github.ReviewComment {
	return github.ReviewComment{
		Identifier: node.ID,
		Author:     node.Author.login(),
		Body:       node.Body,
		CreatedAt:  node.CreatedAt,
		URL:        node.URL,
		DiffHunk:   node.DiffHunk,
		Pending:    node.State == "PENDING",

		ViewerCanUpdate: node.ViewerCanUpdate,
		ViewerCanDelete: node.ViewerCanDelete,
		ViewerDidAuthor: node.ViewerDidAuthor,
		Reactions:       mapReactions(node.ReactionGroups),
	}
}

func mapDetail(node graphqlPullRequestDetail, files []graphqlFile, comments []graphqlIssueComment, reviews []graphqlReview) (github.PullRequestDetail, error) {
	pullRequest, err := mapPullRequest(node.graphqlPullRequest)
	if err != nil {
		return github.PullRequestDetail{}, err
	}
	detail := github.PullRequestDetail{
		PullRequest:     pullRequest,
		ViewerDidAuthor: node.ViewerDidAuthor,
		Body:            node.Body,
		Additions:       node.Additions,
		Deletions:       node.Deletions,
	}
	for _, label := range node.Labels.Nodes {
		detail.Labels = append(detail.Labels, github.Label{Name: label.Name, Color: label.Color})
	}
	for _, record := range node.ReviewRequests.Nodes {
		switch requestedReviewer := record.RequestedReviewer; {
		case requestedReviewer == nil:
		case requestedReviewer.Typename == "Team":
			detail.ReviewRequests = append(detail.ReviewRequests, github.Reviewer{Name: requestedReviewer.Slug, IsTeam: true})
		default:
			detail.ReviewRequests = append(detail.ReviewRequests, github.Reviewer{Name: requestedReviewer.Login})
		}
	}
	for _, file := range files {
		detail.Files = append(detail.Files, github.ChangedFile{
			Path: file.Path, Additions: file.Additions, Deletions: file.Deletions, Change: github.FileChangeType(file.ChangeType), Viewed: github.ViewedState(file.ViewerViewedState),
		})
	}
	for _, graphqlComment := range comments {
		detail.Comments = append(detail.Comments, mapIssueComment(graphqlComment))
	}
	for _, review := range reviews {
		detail.Reviews = append(detail.Reviews, github.Review{
			Identifier: review.ID, Author: review.Author.login(), State: github.ReviewState(review.State), Body: review.Body, SubmittedAt: review.SubmittedAt,
		})
	}
	return detail, nil
}

func integerOrZero(pointer *int) int {
	if pointer == nil {
		return 0
	}
	return *pointer
}

func mapThread(node graphqlThread) github.ReviewThread {
	thread := github.ReviewThread{
		Identifier:        node.ID,
		SubjectType:       github.ThreadSubject(node.SubjectType),
		IsResolved:        node.IsResolved,
		IsOutdated:        node.IsOutdated,
		Path:              node.Path,
		Line:              integerOrZero(node.Line),
		OriginalLine:      integerOrZero(node.OriginalLine),
		StartLine:         integerOrZero(node.StartLine),
		OriginalStartLine: integerOrZero(node.OriginalStartLine),
		DiffSide:          github.DiffSide(node.DiffSide),
		StartDiffSide:     github.DiffSide(node.StartDiffSide),
	}
	if node.ResolvedBy != nil {
		thread.ResolvedBy = node.ResolvedBy.login()
	}
	for _, graphqlComment := range node.Comments.Nodes {
		thread.Comments = append(thread.Comments, mapReviewComment(graphqlComment))
	}
	return thread
}
