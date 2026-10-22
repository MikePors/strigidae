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
	// ID is the GraphQL node ID, needed by mutations that target the pull request.
	ID string
	// HeadSHA is the head commit; review comments are anchored to it, and a
	// change since a diff was loaded means that diff is stale.
	HeadSHA        string
	Repo           Repo
	Number         int
	Title          string
	Author         string
	State          PRState
	Draft          bool
	ReviewDecision ReviewDecision
	// Checks is the combined check state of the head commit.
	Checks CheckState
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
	// Name is the login, or the team slug when IsTeam is set. A team's Name is
	// empty when the token lacks the read:org scope needed to read it.
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

// ViewedState is the viewer's "Viewed" mark on a file of a pull request. It is
// private to the viewer and shared with github.com.
type ViewedState string

// Viewed states. The zero value means the state is unknown and is treated as
// unviewed.
const (
	FileUnviewed ViewedState = "UNVIEWED"
	FileViewed   ViewedState = "VIEWED"
	// FileDismissed means the file was viewed but changed in a later push.
	FileDismissed ViewedState = "DISMISSED"
)

// ChangedFile is one file in a pull request.
type ChangedFile struct {
	Path      string
	Additions int
	Deletions int
	Change    FileChangeType
	// Viewed is the viewer's viewed state of the file.
	Viewed ViewedState
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

// Review is a review summary (the verdict and its optional body). Reviews in
// state ReviewPending are the viewer's own unsubmitted drafts; they stay in
// the list and have a zero SubmittedAt, so sort or filter them deliberately.
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
	// Pending marks a draft comment of the viewer's own pending review, which
	// nobody else can see yet.
	Pending bool
	// DiffHunk is the hunk the comment was made on, useful for outdated threads.
	DiffHunk  string
	Reactions []ReactionSummary
}

// ThreadSubject says what a review thread is attached to.
type ThreadSubject string

// Thread subjects.
const (
	// SubjectLine is a thread on a line or range of lines.
	SubjectLine ThreadSubject = "LINE"
	// SubjectFile is a thread on a whole file; it has no line numbers and is
	// not outdated.
	SubjectFile ThreadSubject = "FILE"
)

// ReviewThread is a conversation anchored to a line of a diff. Threads may
// include the viewer's pending (draft) comments; see ReviewComment.Pending.
type ReviewThread struct {
	// ID is the node ID used to reply to the thread.
	ID          string
	SubjectType ThreadSubject
	IsResolved  bool
	// IsOutdated means the anchored lines changed after the comment was made.
	IsOutdated bool
	ResolvedBy string
	Path       string
	// Line is the line number in the file on DiffSide (the old file for
	// SideLeft, the new file for SideRight), not a position in the diff. It is
	// zero when the thread is outdated and no longer maps onto the current
	// diff (use OriginalLine then), and for file-level threads (SubjectFile).
	Line         int
	OriginalLine int
	// StartLine and OriginalStartLine are set for multi-line threads, else
	// zero. They are on StartDiffSide, which can differ from DiffSide.
	StartLine         int
	OriginalStartLine int
	DiffSide          DiffSide
	StartDiffSide     DiffSide
	Comments          []ReviewComment
}

// ReviewEvent is the verdict of a review being submitted.
type ReviewEvent string

// Review events.
const (
	ReviewEventApprove        ReviewEvent = "APPROVE"
	ReviewEventRequestChanges ReviewEvent = "REQUEST_CHANGES"
	ReviewEventComment        ReviewEvent = "COMMENT"
)

// PendingReview is the viewer's unsubmitted review on a pull request. GitHub
// allows one per user per pull request.
type PendingReview struct {
	ID string
	// Body is the review summary written so far.
	Body string
	// CommentCount is the number of inline comments waiting in it.
	CommentCount int
	// CommitOID is the commit the review is anchored to; empty if unknown.
	CommitOID string
	// CommentIDs are the IDs of its inline comments, as far as the status
	// query lists them (at most 100; compare with CommentCount).
	CommentIDs []string
}

// ReviewStatus is the state a review submission depends on.
type ReviewStatus struct {
	// PullRequestID is the pull request's node ID.
	PullRequestID string
	// HeadSHA is the pull request's current head commit.
	HeadSHA string
	// Pending is the viewer's pending review, or nil.
	Pending *PendingReview
}

// SubmitReviewInput describes a review to submit.
type SubmitReviewInput struct {
	Repo          Repo
	Number        int
	PullRequestID string
	Event         ReviewEvent
	Body          string
	// CommitOID is the commit a new review applies to. A pending review keeps
	// the commit it was started on.
	CommitOID string
	// PendingReviewID, if set, submits that pending review instead of
	// creating a new one.
	PendingReviewID string
}

// StartReviewInput describes a new pending review.
type StartReviewInput struct {
	Repo          Repo
	Number        int
	PullRequestID string
	// CommitOID is the commit the review's comments are anchored to.
	CommitOID string
}

// AddThreadInput describes an inline comment thread to add to a pending
// review. Line is the last line of the range and StartLine the first one for a
// multi-line comment (zero for a single line); both are line numbers on the
// given side of the diff, not positions in it.
type AddThreadInput struct {
	Repo            Repo
	Number          int
	PendingReviewID string
	Path            string
	Line            int
	Side            DiffSide
	StartLine       int
	StartSide       DiffSide
	Body            string
}
