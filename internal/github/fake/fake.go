// Package fake provides an in-memory github.Client for tests and demos. Seed
// it with the Set* methods, call it like the real client, then inspect what
// was recorded with Calls, Replies and Comments.
package fake

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MikePors/strigidae/internal/github"
)

// Method names accepted by SetError and recorded by Calls.
const (
	MethodListPullRequests   = "ListPullRequests"
	MethodSearchPullRequests = "SearchPullRequests"
	MethodGetPullRequest     = "GetPullRequest"
	MethodGetDiff            = "GetDiff"
	MethodListReviewThreads  = "ListReviewThreads"
	MethodReplyToThread      = "ReplyToThread"
	MethodAddComment         = "AddComment"
)

// Reply is a recorded ReplyToThread call.
type Reply struct {
	ThreadID string
	Body     string
}

// Comment is a recorded AddComment call.
type Comment struct {
	Repo   github.Repo
	Number int
	Body   string
}

type prKey struct {
	repo   github.Repo
	number int
}

// Client implements github.Client in memory. The zero value is not usable;
// call New. Cursors are decimal offsets, so Page.Next is stable across calls.
type Client struct {
	// Viewer is the login used as the author of replies and comments.
	Viewer string
	// Now supplies timestamps for created comments.
	Now func() time.Time

	mu       sync.Mutex
	lists    map[github.Repo][]github.PullRequest
	search   []github.PullRequest
	details  map[prKey]github.PullRequestDetail
	diffs    map[prKey]string
	threads  map[prKey][]github.ReviewThread
	errs     map[string]error
	calls    []string
	replies  []Reply
	comments []Comment
	nextID   int
}

var _ github.Client = (*Client)(nil)

// New returns an empty fake client whose viewer is "me".
func New() *Client {
	return &Client{
		Viewer:  "me",
		Now:     time.Now,
		lists:   map[github.Repo][]github.PullRequest{},
		details: map[prKey]github.PullRequestDetail{},
		diffs:   map[prKey]string{},
		threads: map[prKey][]github.ReviewThread{},
		errs:    map[string]error{},
	}
}

// SetPullRequests sets the open pull requests listed for repo.
func (c *Client) SetPullRequests(repo github.Repo, prs []github.PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lists[repo] = append([]github.PullRequest(nil), prs...)
}

// SetSearchResults sets what SearchPullRequests returns, regardless of query.
func (c *Client) SetSearchResults(prs []github.PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.search = append([]github.PullRequest(nil), prs...)
}

// SetPullRequest sets the detail returned for d.Repo and d.Number.
func (c *Client) SetPullRequest(d github.PullRequestDetail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.details[prKey{d.Repo, d.Number}] = d
}

// SetDiff sets the raw diff returned for a pull request.
func (c *Client) SetDiff(repo github.Repo, number int, diff string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diffs[prKey{repo, number}] = diff
}

// SetReviewThreads sets the threads returned for a pull request.
func (c *Client) SetReviewThreads(repo github.Repo, number int, threads []github.ReviewThread) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.threads[prKey{repo, number}] = append([]github.ReviewThread(nil), threads...)
}

// SetError makes the named method fail with err until cleared with a nil err.
// An empty method applies to every method that has no error of its own.
func (c *Client) SetError(method string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		delete(c.errs, method)
		return
	}
	c.errs[method] = err
}

// Calls returns the names of the methods called so far, in order.
func (c *Client) Calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

// Replies returns the recorded successful ReplyToThread calls.
func (c *Client) Replies() []Reply {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Reply(nil), c.replies...)
}

// Comments returns the recorded successful AddComment calls.
func (c *Client) Comments() []Comment {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Comment(nil), c.comments...)
}

// begin records the call and returns the injected error, if any. Callers hold c.mu.
func (c *Client) begin(ctx context.Context, method string) error {
	c.calls = append(c.calls, method)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err, ok := c.errs[method]; ok {
		return err
	}
	return c.errs[""]
}

func paginate[T any](items []T, p github.PageRequest) (github.Page[T], error) {
	start := 0
	if p.After != "" {
		n, err := strconv.Atoi(p.After)
		if err != nil || n < 0 || n > len(items) {
			return github.Page[T]{}, fmt.Errorf("invalid cursor %q", p.After)
		}
		start = n
	}
	end := min(start+p.Size(), len(items))
	page := github.Page[T]{Items: append([]T(nil), items[start:end]...), Total: len(items)}
	if end < len(items) {
		page.Next = strconv.Itoa(end)
	}
	return page, nil
}

// ListPullRequests implements github.Client.
func (c *Client) ListPullRequests(ctx context.Context, repo github.Repo, page github.PageRequest) (github.Page[github.PullRequest], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, MethodListPullRequests); err != nil {
		return github.Page[github.PullRequest]{}, err
	}
	prs, ok := c.lists[repo]
	if !ok {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repo, github.ErrNotFound)
	}
	return paginate(prs, page)
}

// SearchPullRequests implements github.Client.
func (c *Client) SearchPullRequests(ctx context.Context, _ string, page github.PageRequest) (github.Page[github.PullRequest], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, MethodSearchPullRequests); err != nil {
		return github.Page[github.PullRequest]{}, err
	}
	return paginate(c.search, page)
}

// GetPullRequest implements github.Client.
func (c *Client) GetPullRequest(ctx context.Context, repo github.Repo, number int) (github.PullRequestDetail, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, MethodGetPullRequest); err != nil {
		return github.PullRequestDetail{}, err
	}
	d, ok := c.details[prKey{repo, number}]
	if !ok {
		return github.PullRequestDetail{}, fmt.Errorf("load pull request %s#%d: %w", repo, number, github.ErrNotFound)
	}
	return d, nil
}

// GetDiff implements github.Client.
func (c *Client) GetDiff(ctx context.Context, repo github.Repo, number int) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, MethodGetDiff); err != nil {
		return "", err
	}
	d, ok := c.diffs[prKey{repo, number}]
	if !ok {
		return "", fmt.Errorf("load diff of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	return d, nil
}

// ListReviewThreads implements github.Client. A pull request with no threads
// set yields an empty page.
func (c *Client) ListReviewThreads(ctx context.Context, repo github.Repo, number int, page github.PageRequest) (github.Page[github.ReviewThread], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, MethodListReviewThreads); err != nil {
		return github.Page[github.ReviewThread]{}, err
	}
	return paginate(c.threads[prKey{repo, number}], page)
}

// ReplyToThread implements github.Client. The reply is appended to the thread
// so later ListReviewThreads calls include it.
func (c *Client) ReplyToThread(ctx context.Context, threadID, body string) (github.ReviewComment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, MethodReplyToThread); err != nil {
		return github.ReviewComment{}, err
	}
	for key, threads := range c.threads {
		for i := range threads {
			if threads[i].ID != threadID {
				continue
			}
			c.nextID++
			rc := github.ReviewComment{
				ID:        "fake-review-comment-" + strconv.Itoa(c.nextID),
				Author:    c.Viewer,
				Body:      body,
				CreatedAt: c.Now(),
			}
			threads[i].Comments = append(threads[i].Comments, rc)
			c.threads[key] = threads
			c.replies = append(c.replies, Reply{ThreadID: threadID, Body: body})
			return rc, nil
		}
	}
	return github.ReviewComment{}, fmt.Errorf("reply to thread %s: %w", threadID, github.ErrNotFound)
}

// AddComment implements github.Client. The comment is appended to the pull
// request's detail if one was set.
func (c *Client) AddComment(ctx context.Context, repo github.Repo, number int, body string) (github.IssueComment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, MethodAddComment); err != nil {
		return github.IssueComment{}, err
	}
	if strings.TrimSpace(body) == "" {
		return github.IssueComment{}, fmt.Errorf("add comment to %s#%d: empty body", repo, number)
	}
	c.nextID++
	ic := github.IssueComment{
		ID:        "fake-comment-" + strconv.Itoa(c.nextID),
		Author:    c.Viewer,
		Body:      body,
		CreatedAt: c.Now(),
	}
	key := prKey{repo, number}
	if d, ok := c.details[key]; ok {
		d.Comments = append(d.Comments, ic)
		d.CommentCount = len(d.Comments)
		c.details[key] = d
	}
	c.comments = append(c.comments, Comment{Repo: repo, Number: number, Body: body})
	return ic, nil
}
