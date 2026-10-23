package network

import (
	"time"

	"github.com/MikePors/strigidae/internal/github"
)

// ghost is shown for authors whose account was deleted (GraphQL returns null).
const ghost = "ghost"

type graphqlActor struct{ Login string }

func (a *graphqlActor) login() string {
	if a == nil || a.Login == "" {
		return ghost
	}
	return a.Login
}

type graphqlPageInfo struct {
	HasNextPage bool
	EndCursor   string
}

func (p graphqlPageInfo) next() string {
	if !p.HasNextPage {
		return ""
	}
	return p.EndCursor
}

type graphqlConnection[N any] struct {
	TotalCount int
	PageInfo   graphqlPageInfo
	Nodes      []N
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
	Body      string
	Additions int
	Deletions int
	Labels    struct {
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

func mapPullRequest(n graphqlPullRequest) (github.PullRequest, error) {
	repo, err := github.ParseRepository(n.Repository.NameWithOwner)
	if err != nil {
		return github.PullRequest{}, err
	}
	return github.PullRequest{
		Identifier:     n.ID,
		HeadSHA:        n.HeadRefOid,
		Repository:     repo,
		Number:         n.Number,
		Title:          n.Title,
		Author:         n.Author.login(),
		State:          github.PullRequestState(n.State),
		Draft:          n.IsDraft,
		ReviewDecision: github.ReviewDecision(n.ReviewDecision),
		Checks:         mapChecks(n),
		CommentCount:   n.CommentCount.TotalCount,
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
		URL:            n.URL,
		HeadBranch:     n.HeadRefName,
		BaseBranch:     n.BaseRefName,
	}, nil
}

// mapChecks reads the head commit's rollup; null means it has no checks.
func mapChecks(n graphqlPullRequest) github.CheckState {
	if n.StatusCheckRollup == nil {
		return github.ChecksNone
	}
	return github.CheckStateFromRollup(n.StatusCheckRollup.State)
}

func mapPullRequests(nodes []graphqlPullRequest) ([]github.PullRequest, error) {
	out := make([]github.PullRequest, 0, len(nodes))
	for _, n := range nodes {
		// Search results can contain nulls for inaccessible items, which decode
		// to zero values.
		if n.Number == 0 {
			continue
		}
		pr, err := mapPullRequest(n)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, nil
}

func mapReactions(groups []graphqlReactionGroup) []github.ReactionSummary {
	var out []github.ReactionSummary
	for _, g := range groups {
		if g.Reactors.TotalCount == 0 {
			continue
		}
		out = append(out, github.ReactionSummary{
			Content:       github.ReactionContent(g.Content),
			Count:         g.Reactors.TotalCount,
			ViewerReacted: g.ViewerHasReacted,
		})
	}
	return out
}

func mapIssueComment(n graphqlIssueComment) github.IssueComment {
	return github.IssueComment{
		Identifier: n.ID,
		Author:     n.Author.login(),
		Body:       n.Body,
		CreatedAt:  n.CreatedAt,
		URL:        n.URL,
		Reactions:  mapReactions(n.ReactionGroups),
	}
}

func mapReviewComment(n graphqlReviewComment) github.ReviewComment {
	return github.ReviewComment{
		Identifier: n.ID,
		Author:     n.Author.login(),
		Body:       n.Body,
		CreatedAt:  n.CreatedAt,
		URL:        n.URL,
		DiffHunk:   n.DiffHunk,
		Pending:    n.State == "PENDING",

		ViewerCanUpdate: n.ViewerCanUpdate,
		ViewerCanDelete: n.ViewerCanDelete,
		ViewerDidAuthor: n.ViewerDidAuthor,
		Reactions:       mapReactions(n.ReactionGroups),
	}
}

func mapDetail(n graphqlPullRequestDetail, files []graphqlFile, comments []graphqlIssueComment, reviews []graphqlReview) (github.PullRequestDetail, error) {
	pr, err := mapPullRequest(n.graphqlPullRequest)
	if err != nil {
		return github.PullRequestDetail{}, err
	}
	d := github.PullRequestDetail{
		PullRequest: pr,
		Body:        n.Body,
		Additions:   n.Additions,
		Deletions:   n.Deletions,
	}
	for _, l := range n.Labels.Nodes {
		d.Labels = append(d.Labels, github.Label{Name: l.Name, Color: l.Color})
	}
	for _, r := range n.ReviewRequests.Nodes {
		switch rr := r.RequestedReviewer; {
		case rr == nil:
		case rr.Typename == "Team":
			d.ReviewRequests = append(d.ReviewRequests, github.Reviewer{Name: rr.Slug, IsTeam: true})
		default:
			d.ReviewRequests = append(d.ReviewRequests, github.Reviewer{Name: rr.Login})
		}
	}
	for _, f := range files {
		d.Files = append(d.Files, github.ChangedFile{
			Path: f.Path, Additions: f.Additions, Deletions: f.Deletions, Change: github.FileChangeType(f.ChangeType), Viewed: github.ViewedState(f.ViewerViewedState),
		})
	}
	for _, c := range comments {
		d.Comments = append(d.Comments, mapIssueComment(c))
	}
	for _, r := range reviews {
		d.Reviews = append(d.Reviews, github.Review{
			Identifier: r.ID, Author: r.Author.login(), State: github.ReviewState(r.State), Body: r.Body, SubmittedAt: r.SubmittedAt,
		})
	}
	return d, nil
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func mapThread(n graphqlThread) github.ReviewThread {
	t := github.ReviewThread{
		Identifier:        n.ID,
		SubjectType:       github.ThreadSubject(n.SubjectType),
		IsResolved:        n.IsResolved,
		IsOutdated:        n.IsOutdated,
		Path:              n.Path,
		Line:              intOrZero(n.Line),
		OriginalLine:      intOrZero(n.OriginalLine),
		StartLine:         intOrZero(n.StartLine),
		OriginalStartLine: intOrZero(n.OriginalStartLine),
		DiffSide:          github.DiffSide(n.DiffSide),
		StartDiffSide:     github.DiffSide(n.StartDiffSide),
	}
	if n.ResolvedBy != nil {
		t.ResolvedBy = n.ResolvedBy.login()
	}
	for _, c := range n.Comments.Nodes {
		t.Comments = append(t.Comments, mapReviewComment(c))
	}
	return t
}
