package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"

	"github.com/MikePors/strigidae/internal/github"
)

// prQualifiers are the search tokens that already restrict results to pull
// requests. Whole tokens are compared: "is:private" and "-is:pr" do not count.
var prQualifiers = []string{"is:pr", "type:pr", "is:pull-request", "type:pull-request"}

func hasPRQualifier(query string) bool {
	for _, f := range strings.Fields(query) {
		if slices.Contains(prQualifiers, f) {
			return true
		}
	}
	return false
}

func afterVar(p github.PageRequest) any {
	if p.After == "" {
		return nil
	}
	return p.After
}

// ListPullRequests implements github.Client.
func (c *Client) ListPullRequests(ctx context.Context, repo github.Repo, page github.PageRequest) (github.Page[github.PullRequest], error) {
	var resp struct {
		Repository *struct {
			PullRequests gqlConn[gqlPullRequest]
		}
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "first": page.Size(), "after": afterVar(page)}
	if err := c.query(ctx, listPullRequestsQuery, vars, &resp); err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repo, err)
	}
	if resp.Repository == nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repo, github.ErrNotFound)
	}
	conn := resp.Repository.PullRequests
	items, err := mapPullRequests(conn.Nodes)
	if err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repo, err)
	}
	return github.Page[github.PullRequest]{Items: items, Next: conn.PageInfo.next(), Total: conn.TotalCount}, nil
}

// SearchPullRequests implements github.Client.
func (c *Client) SearchPullRequests(ctx context.Context, query string, page github.PageRequest) (github.Page[github.PullRequest], error) {
	var resp struct {
		Search struct {
			IssueCount int
			PageInfo   gqlPageInfo
			Nodes      []gqlPullRequest
		}
	}
	q := query
	if !hasPRQualifier(q) {
		q = "is:pr " + q
	}
	vars := map[string]any{"query": q, "first": page.Size(), "after": afterVar(page)}
	if err := c.query(ctx, searchPullRequestsQuery, vars, &resp); err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("search pull requests: %w", err)
	}
	items, err := mapPullRequests(resp.Search.Nodes)
	if err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("search pull requests: %w", err)
	}
	return github.Page[github.PullRequest]{Items: items, Next: resp.Search.PageInfo.next(), Total: resp.Search.IssueCount}, nil
}

// GetPullRequest implements github.Client. Files, comments and reviews beyond
// the first 100 are fetched with follow-up requests.
func (c *Client) GetPullRequest(ctx context.Context, repo github.Repo, number int) (github.PullRequestDetail, error) {
	d, err := c.getPullRequest(ctx, repo, number)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("load pull request %s#%d: %w", repo, number, err)
	}
	return d, nil
}

func (c *Client) getPullRequest(ctx context.Context, repo github.Repo, number int) (github.PullRequestDetail, error) {
	var resp struct {
		Repository *struct {
			PullRequest *gqlPullRequestDetail
		}
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	err := c.query(ctx, detailQuery(true), vars, &resp)
	if isScopeError(err) {
		// Team details need read:org. Retry once without them rather than
		// failing a pull request the user can otherwise read.
		resp.Repository = nil
		err = c.query(ctx, detailQuery(false), vars, &resp)
	}
	if err != nil {
		return github.PullRequestDetail{}, err
	}
	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return github.PullRequestDetail{}, github.ErrNotFound
	}
	n := resp.Repository.PullRequest

	files, err := collectMore(ctx, c, repo, number, moreFilesQuery, n.Files)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("files: %w", err)
	}
	comments, err := collectMore(ctx, c, repo, number, moreCommentsQuery, n.Comments)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("comments: %w", err)
	}
	reviews, err := collectMore(ctx, c, repo, number, moreReviewsQuery, n.Reviews)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("reviews: %w", err)
	}
	return mapDetail(*n, files, comments, reviews)
}

// collectMore returns all nodes of a connection, following its cursor.
func collectMore[N any](ctx context.Context, c *Client, repo github.Repo, number int, query string, first gqlConn[N]) ([]N, error) {
	nodes := first.Nodes
	cursor := first.PageInfo.next()
	seen := map[string]bool{}
	for cursor != "" {
		if seen[cursor] {
			return nil, fmt.Errorf("pagination did not advance (repeated cursor %q)", cursor)
		}
		seen[cursor] = true
		var resp struct {
			Repository *struct {
				PullRequest *struct{ Conn gqlConn[N] }
			}
		}
		vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number, "after": cursor}
		if err := c.query(ctx, query, vars, &resp); err != nil {
			return nil, err
		}
		if resp.Repository == nil || resp.Repository.PullRequest == nil {
			return nil, github.ErrNotFound
		}
		conn := resp.Repository.PullRequest.Conn
		nodes = append(nodes, conn.Nodes...)
		cursor = conn.PageInfo.next()
	}
	return nodes, nil
}

// GetDiff implements github.Client.
func (c *Client) GetDiff(ctx context.Context, repo github.Repo, number int) (string, error) {
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	resp, err := c.diff.RequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		var httpErr *ghapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotAcceptable {
			return "", fmt.Errorf("load diff of %s#%d: %w: %w", repo, number, github.ErrDiffTooLarge, err)
		}
		return "", fmt.Errorf("load diff of %s#%d: %w", repo, number, c.mapError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read diff of %s#%d: %w", repo, number, err)
	}
	return string(b), nil
}

// ListReviewThreads implements github.Client.
func (c *Client) ListReviewThreads(ctx context.Context, repo github.Repo, number int, page github.PageRequest) (github.Page[github.ReviewThread], error) {
	var resp struct {
		Repository *struct {
			PullRequest *struct{ ReviewThreads gqlConn[gqlThread] }
		}
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number, "first": page.Size(), "after": afterVar(page)}
	if err := c.query(ctx, reviewThreadsQuery, vars, &resp); err != nil {
		return github.Page[github.ReviewThread]{}, fmt.Errorf("list review threads of %s#%d: %w", repo, number, err)
	}
	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return github.Page[github.ReviewThread]{}, fmt.Errorf("list review threads of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	conn := resp.Repository.PullRequest.ReviewThreads
	items := make([]github.ReviewThread, 0, len(conn.Nodes))
	for _, n := range conn.Nodes {
		items = append(items, mapThread(n))
	}
	return github.Page[github.ReviewThread]{Items: items, Next: conn.PageInfo.next(), Total: conn.TotalCount}, nil
}

// ReplyToThread implements github.Client.
func (c *Client) ReplyToThread(ctx context.Context, threadID, body string) (github.ReviewComment, error) {
	var resp struct {
		AddPullRequestReviewThreadReply *struct{ Comment *gqlReviewComment }
	}
	vars := map[string]any{"threadId": threadID, "body": body}
	if err := c.query(ctx, replyToThreadMutation, vars, &resp); err != nil {
		return github.ReviewComment{}, fmt.Errorf("reply to thread: %w", err)
	}
	if resp.AddPullRequestReviewThreadReply == nil || resp.AddPullRequestReviewThreadReply.Comment == nil {
		return github.ReviewComment{}, fmt.Errorf("reply to thread: %w", github.ErrNotFound)
	}
	return mapReviewComment(*resp.AddPullRequestReviewThreadReply.Comment), nil
}

// AddComment implements github.Client. It uses REST because that call needs
// only the repository and number, not the pull request's node ID.
func (c *Client) AddComment(ctx context.Context, repo github.Repo, number int, body string) (github.IssueComment, error) {
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return github.IssueComment{}, fmt.Errorf("add comment: %w", err)
	}
	var resp struct {
		NodeID    string `json:"node_id"`
		HTMLURL   string `json:"html_url"`
		Body      string
		CreatedAt time.Time `json:"created_at"`
		User      *gqlActor
	}
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	if err := c.rest.DoWithContext(ctx, "POST", path, bytes.NewReader(payload), &resp); err != nil {
		return github.IssueComment{}, fmt.Errorf("add comment to %s#%d: %w", repo, number, c.mapError(err))
	}
	return github.IssueComment{ID: resp.NodeID, Author: resp.User.login(), Body: resp.Body, CreatedAt: resp.CreatedAt, URL: resp.HTMLURL}, nil
}

// SetFileViewed implements github.Client.
func (c *Client) SetFileViewed(ctx context.Context, pullRequestID, path string, viewed bool) error {
	q := unmarkFileViewedMutation
	if viewed {
		q = markFileViewedMutation
	}
	var resp json.RawMessage
	// The caller names the action and the file, so the error is not wrapped again.
	return c.query(ctx, q, map[string]any{"pullRequestId": pullRequestID, "path": path}, &resp)
}
