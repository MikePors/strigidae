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
	return func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
		return true
	}
}

func TestGetReviewStatusNoPendingReview(test *testing.T) {
	testServer := &server{override: respondWith(`{"data":{"repository":{"pullRequest":{"id":"PR_1","headRefOid":"abc","reviews":{"nodes":[]}}}}}`)}
	client := newTestClient(test, testServer)
	got, err := client.GetReviewStatus(context.Background(), repository, 27)
	if err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff(github.ReviewStatus{PullRequestIdentifier: "PR_1", HeadSHA: "abc"}, got); diff != "" {
		test.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestGetReviewStatusNotFound(test *testing.T) {
	for name, body := range map[string]string{
		"null repository":   `{"data":{"repository":null}}`,
		"null pull request": `{"data":{"repository":{"pullRequest":null}}}`,
	} {
		test.Run(name, func(test *testing.T) {
			client := newTestClient(test, &server{override: respondWith(body)})
			_, err := client.GetReviewStatus(context.Background(), repository, 27)
			if !errors.Is(err, github.ErrNotFound) {
				test.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
}
