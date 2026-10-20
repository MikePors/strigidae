package github

import "time"

// PRState is the lifecycle state of a pull request.
type PRState string

// Pull request states.
const (
	PRStateOpen   PRState = "OPEN"
	PRStateClosed PRState = "CLOSED"
	PRStateMerged PRState = "MERGED"
)

// ReviewDecision is the aggregate review outcome. Empty means no decision
// (reviews not required, or none submitted).
type ReviewDecision string

// Review decisions.
const (
	ReviewDecisionNone             ReviewDecision = ""
	ReviewDecisionApproved         ReviewDecision = "APPROVED"
	ReviewDecisionChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	ReviewDecisionReviewRequired   ReviewDecision = "REVIEW_REQUIRED"
)

// PullRequest is the summary shown in lists.
type PullRequest struct {
	Repo           Repo
	Number         int
	Title          string
	Author         string
	State          PRState
	Draft          bool
	ReviewDecision ReviewDecision
	// CommentCount counts top-level (conversation) comments only.
	CommentCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	URL          string
	HeadRef      string
	BaseRef      string
}

// Label is a repository label.
type Label struct {
	Name string
	// Color is a hex RGB value without the leading "#".
	Color string
}

// Reviewer is a user or team whose review was requested.
type Reviewer struct {
	// Name is the login, or the team slug when IsTeam is set.
	Name   string
	IsTeam bool
}

// FileChangeType describes how a file changed.
type FileChangeType string

// File change types.
const (
	FileAdded    FileChangeType = "ADDED"
	FileModified FileChangeType = "MODIFIED"
	FileDeleted  FileChangeType = "DELETED"
	FileRenamed  FileChangeType = "RENAMED"
	FileCopied   FileChangeType = "COPIED"
	FileChanged  FileChangeType = "CHANGED"
)

// ChangedFile is one file in a pull request.
type ChangedFile struct {
	Path      string
	Additions int
	Deletions int
	Change    FileChangeType
}

// ReviewState is the state of a submitted review.
type ReviewState string

// Review states.
const (
	ReviewApproved         ReviewState = "APPROVED"
	ReviewChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewCommented        ReviewState = "COMMENTED"
	ReviewDismissed        ReviewState = "DISMISSED"
	ReviewPending          ReviewState = "PENDING"
)

// Review is a review summary (the verdict and its optional body).
type Review struct {
	ID          string
	Author      string
	State       ReviewState
	Body        string
	SubmittedAt time.Time
}

// PullRequestDetail is a pull request with everything the detail view needs.
type PullRequestDetail struct {
	PullRequest
	Body      string
	Additions int
	Deletions int
	Labels    []Label
	// ReviewRequests are the reviewers currently asked to review.
	ReviewRequests []Reviewer
	Files          []ChangedFile
	// Comments are the top-level conversation comments, oldest first.
	Comments []IssueComment
	// Reviews are submitted review summaries, oldest first.
	Reviews []Review
}

// ReactionContent is a kind of emoji reaction.
type ReactionContent string

// Reaction kinds.
const (
	ReactionThumbsUp   ReactionContent = "THUMBS_UP"
	ReactionThumbsDown ReactionContent = "THUMBS_DOWN"
	ReactionLaugh      ReactionContent = "LAUGH"
	ReactionHooray     ReactionContent = "HOORAY"
	ReactionConfused   ReactionContent = "CONFUSED"
	ReactionHeart      ReactionContent = "HEART"
	ReactionRocket     ReactionContent = "ROCKET"
	ReactionEyes       ReactionContent = "EYES"
)

// Emoji returns the emoji for the reaction, or the raw content if unknown.
func (c ReactionContent) Emoji() string {
	switch c {
	case ReactionThumbsUp:
		return "👍"
	case ReactionThumbsDown:
		return "👎"
	case ReactionLaugh:
		return "😄"
	case ReactionHooray:
		return "🎉"
	case ReactionConfused:
		return "😕"
	case ReactionHeart:
		return "❤️"
	case ReactionRocket:
		return "🚀"
	case ReactionEyes:
		return "👀"
	default:
		return string(c)
	}
}

// ReactionSummary is the count of one kind of reaction on a comment.
type ReactionSummary struct {
	Content       ReactionContent
	Count         int
	ViewerReacted bool
}

// IssueComment is a top-level comment on a pull request.
type IssueComment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
	URL       string
	Reactions []ReactionSummary
}

// DiffSide says which side of a split diff a thread line refers to.
type DiffSide string

// Diff sides.
const (
	// SideLeft is the base (removed) side.
	SideLeft DiffSide = "LEFT"
	// SideRight is the head (added) side.
	SideRight DiffSide = "RIGHT"
)

// ReviewComment is a comment inside a review thread.
type ReviewComment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
	URL       string
	// DiffHunk is the hunk the comment was made on, useful for outdated threads.
	DiffHunk  string
	Reactions []ReactionSummary
}

// ReviewThread is a conversation anchored to a line of a diff.
type ReviewThread struct {
	// ID is the node ID used to reply to the thread.
	ID         string
	IsResolved bool
	// IsOutdated means the anchored lines changed after the comment was made.
	IsOutdated bool
	ResolvedBy string
	Path       string
	// Line is the line in the current diff; zero when the thread is outdated
	// and no longer maps onto it. Use OriginalLine then.
	Line         int
	OriginalLine int
	// StartLine and OriginalStartLine are set for multi-line threads, else zero.
	StartLine         int
	OriginalStartLine int
	DiffSide          DiffSide
	Comments          []ReviewComment
}
