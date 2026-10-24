package fake

import (
	"context"
	"fmt"
	"slices"

	"github.com/MikePors/strigidae/internal/github"
)

// Method names recorded in Call.Method for resolving; Call.ThreadIdentifier
// holds the thread.
const (
	MethodResolveReviewThread   = "ResolveReviewThread"
	MethodUnresolveReviewThread = "UnresolveReviewThread"
)

// ResolveReviewThread implements github.Client. Resolving a resolved thread
// succeeds and changes nothing, as on GitHub.
func (client *Client) ResolveReviewThread(ctx context.Context, threadIdentifier string) (github.ThreadResolution, error) {
	return client.setResolved(ctx, MethodResolveReviewThread, threadIdentifier, true)
}

// UnresolveReviewThread implements github.Client.
func (client *Client) UnresolveReviewThread(ctx context.Context, threadIdentifier string) (github.ThreadResolution, error) {
	return client.setResolved(ctx, MethodUnresolveReviewThread, threadIdentifier, false)
}

func (client *Client) setResolved(ctx context.Context, method, threadIdentifier string, resolved bool) (github.ThreadResolution, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: method, ThreadIdentifier: threadIdentifier}); err != nil {
		return github.ThreadResolution{}, err
	}
	for key, threads := range client.threads {
		index := slices.IndexFunc(threads, func(thread github.ReviewThread) bool { return thread.Identifier == threadIdentifier })
		if index < 0 {
			continue
		}
		// Build a fresh slice so slices handed out earlier are never written to.
		threads = slices.Clone(threads)
		threads[index].IsResolved = resolved
		threads[index].ResolvedBy = ""
		if resolved {
			threads[index].ResolvedBy = client.Viewer
		}
		client.threads[key] = threads
		return github.ThreadResolution{Identifier: threadIdentifier, IsResolved: resolved, ResolvedBy: threads[index].ResolvedBy}, nil
	}
	return github.ThreadResolution{}, fmt.Errorf("thread %s: %w", threadIdentifier, github.ErrNotFound)
}
