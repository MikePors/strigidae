package network

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func respondWith(body string) func(call, http.ResponseWriter) bool {
	return func(_ call, w http.ResponseWriter) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
		return true
	}
}

func TestGetReviewStatusNoPendingReview(t *testing.T) {
	s := &server{override: respondWith(`{"data":{"repository":{"pullRequest":{"id":"PR_1","headRefOid":"abc","reviews":{"nodes":[]}}}}}`)}
	c := newTestClient(t, s)
	got, err := c.GetReviewStatus(context.Background(), repo, 27)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(github.ReviewStatus{PullRequestID: "PR_1", HeadSHA: "abc"}, got); diff != "" {
		t.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestGetReviewStatusNotFound(t *testing.T) {
	for name, body := range map[string]string{
		"null repository":   `{"data":{"repository":null}}`,
		"null pull request": `{"data":{"repository":{"pullRequest":null}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, &server{override: respondWith(body)})
			_, err := c.GetReviewStatus(context.Background(), repo, 27)
			if !errors.Is(err, github.ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
}
