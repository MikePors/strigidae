package api

import (
	"time"

	"github.com/MikePors/strigidae/internal/github"
)

// ghost is shown for authors whose account was deleted (GraphQL returns null).
const ghost = "ghost"

type gqlActor struct{ Login string }

func (a *gqlActor) login() string {
	if a == nil || a.Login == "" {
		return ghost
	}
	return a.Login
}

type gqlPageInfo struct {
	HasNextPage bool
	EndCursor   string
}

func (p gqlPageInfo) next() string {
	if !p.HasNextPage {
		return ""
	}
	return p.EndCursor
}

type gqlConn[N any] struct {
	TotalCount int
	PageInfo   gqlPageInfo
	Nodes      []N
}

type gqlPullRequest struct {
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
	Author            *gqlActor
	CommentCount      struct{ TotalCount int }
	Repository        struct{ NameWithOwner string }
}

type gqlReactionGroup struct {
	Content          string
	ViewerHasReacted bool
	Reactors         struct{ TotalCount int }
}

type gqlIssueComment struct {
	ID             string
	URL            string
	Body           string
	CreatedAt      time.Time
	Author         *gqlActor
	ReactionGroups []gqlReactionGroup
}

type gqlReviewComment struct {
	ID             string
	State          string
	URL            string
	Body           string
	DiffHunk       string
	CreatedAt      time.Time
	Author         *gqlActor
	ReactionGroups []gqlReactionGroup

	ViewerCanUpdate bool
	ViewerCanDelete bool
}

type gqlFile struct {
	Path       string
	Additions  int
	Deletions  int
	ChangeType string
	// ViewerViewedState is VIEWED, UNVIEWED or DISMISSED.
	ViewerViewedState string
}

type gqlReview struct {
	ID          string
	State       string
	Body        string
	SubmittedAt time.Time
	Author      *gqlActor
}

type gqlPullRequestDetail struct {
	gqlPullRequest
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
	Files    gqlConn[gqlFile]
	Comments gqlConn[gqlIssueComment]
	Reviews  gqlConn[gqlReview]
}

type gqlThread struct {
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
	ResolvedBy        *gqlActor
	Comments          struct{ Nodes []gqlReviewComment }
}

func mapPullRequest(n gqlPullRequest) (github.PullRequest, error) {
	repo, err := github.ParseRepo(n.Repository.NameWithOwner)
	if err != nil {
		return github.PullRequest{}, err
	}
	return github.PullRequest{
		ID:             n.ID,
		HeadSHA:        n.HeadRefOid,
		Repo:           repo,
		Number:         n.Number,
		Title:          n.Title,
		Author:         n.Author.login(),
		State:          github.PRState(n.State),
		Draft:          n.IsDraft,
		ReviewDecision: github.ReviewDecision(n.ReviewDecision),
		Checks:         mapChecks(n),
		CommentCount:   n.CommentCount.TotalCount,
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
		URL:            n.URL,
		HeadRef:        n.HeadRefName,
		BaseRef:        n.BaseRefName,
	}, nil
}

// mapChecks reads the head commit's rollup; null means it has no checks.
func mapChecks(n gqlPullRequest) github.CheckState {
	if n.StatusCheckRollup == nil {
		return github.ChecksNone
	}
	return github.CheckStateFromRollup(n.StatusCheckRollup.State)
}

func mapPullRequests(nodes []gqlPullRequest) ([]github.PullRequest, error) {
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

func mapReactions(groups []gqlReactionGroup) []github.ReactionSummary {
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

func mapIssueComment(n gqlIssueComment) github.IssueComment {
	return github.IssueComment{
		ID:        n.ID,
		Author:    n.Author.login(),
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
		URL:       n.URL,
		Reactions: mapReactions(n.ReactionGroups),
	}
}

func mapReviewComment(n gqlReviewComment) github.ReviewComment {
	return github.ReviewComment{
		ID:        n.ID,
		Author:    n.Author.login(),
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
		URL:       n.URL,
		DiffHunk:  n.DiffHunk,
		Pending:   n.State == "PENDING",

		ViewerCanUpdate: n.ViewerCanUpdate,
		ViewerCanDelete: n.ViewerCanDelete,
		Reactions:       mapReactions(n.ReactionGroups),
	}
}

func mapDetail(n gqlPullRequestDetail, files []gqlFile, comments []gqlIssueComment, reviews []gqlReview) (github.PullRequestDetail, error) {
	pr, err := mapPullRequest(n.gqlPullRequest)
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
			ID: r.ID, Author: r.Author.login(), State: github.ReviewState(r.State), Body: r.Body, SubmittedAt: r.SubmittedAt,
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

func mapThread(n gqlThread) github.ReviewThread {
	t := github.ReviewThread{
		ID:                n.ID,
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
