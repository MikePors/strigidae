package network

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestViewerReviewedSHAIsMappedFromTheLatestSubmittedReview(test *testing.T) {
	testServer := &server{}
	testServer.override = func(call call, writer http.ResponseWriter) bool {
		if call.Operation != operationPullRequestList {
			return false
		}
		testServer.fixture(writer, http.StatusOK, "PullRequestList.viewer_reviews.json")
		return true
	}
	client := newTestClient(test, testServer)
	page, err := client.ListPullRequests(context.Background(), repository, github.PageRequest{})
	if err != nil {
		test.Fatal(err)
	}
	head := func(number int) string { return fmt.Sprintf("%040d", number) }
	want := []struct {
		title    string
		headSHA  string
		reviewed string
	}{
		{"reviewed at head", head(11), head(11)},
		{"reviewed at an older commit", head(12), head(99)},
		{"never reviewed", head(13), ""},
		{"only a pending review", head(14), ""},
		{"dismissed review at an older commit", head(15), head(98)},
	}
	var got []struct {
		title    string
		headSHA  string
		reviewed string
	}
	for _, pullRequest := range page.Items {
		got = append(got, struct {
			title    string
			headSHA  string
			reviewed string
		}{pullRequest.Title, pullRequest.HeadSHA, pullRequest.ViewerReviewedSHA})
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(want[0])); diff != "" {
		test.Errorf("viewer reviewed commits mismatch (-want +got):\n%s", diff)
	}
}
