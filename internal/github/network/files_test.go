package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

const filesPath = "/repos/MikePors/strigidae/pulls/27/files"

func TestListFilePatches(test *testing.T) {
	testServer := &server{}
	testServer.override = func(recordedCall call, writer http.ResponseWriter) bool {
		if recordedCall.Path != filesPath {
			return false
		}
		testServer.fixture(writer, http.StatusOK, "pull_request_files.json")
		return true
	}
	client := newTestClient(test, testServer)
	got, err := client.ListFilePatches(context.Background(), repository, 27)
	if err != nil {
		test.Fatal(err)
	}
	want := []github.FilePatch{
		{Path: "internal/diff/diff.go", Change: github.FileModified, Additions: 1, Deletions: 1, Patch: "@@ -1,3 +1,3 @@\n package diff\n-// old\n+// new\n \n"},
		{Path: "docs/new.md", Change: github.FileAdded, Additions: 1, Patch: "@@ -0,0 +1 @@\n+# New"},
		{Path: "docs/after.md", PreviousPath: "docs/before.md", Change: github.FileRenamed},
		{Path: "assets/logo.png", Change: github.FileDeleted},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("mismatch (-want +got):\n%s", diff)
	}
	calls := testServer.Calls()
	if len(calls) != 1 {
		test.Fatalf("a short first page should be the only request, got %d", len(calls))
	}
	if query, _ := url.ParseQuery(calls[0].RawQuery); query.Get("per_page") != "100" || query.Get("page") != "1" {
		test.Errorf("query = %q", calls[0].RawQuery)
	}
}

// fullPages serves pages of 100 files up to pages, then an empty one.
func fullPages(testServer *server, pages int) func(call, http.ResponseWriter) bool {
	return func(recordedCall call, writer http.ResponseWriter) bool {
		query, _ := url.ParseQuery(recordedCall.RawQuery)
		var page int
		_, _ = fmt.Sscan(query.Get("page"), &page)
		files := []restFile{}
		if page <= pages {
			for i := range filePageSize {
				files = append(files, restFile{Filename: fmt.Sprintf("file%d-%d.go", page, i), Status: "modified"})
			}
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(writer).Encode(files); err != nil {
			testServer.test.Error(err)
		}
		return true
	}
}

func TestListFilePatchesPages(test *testing.T) {
	for name, testCase := range map[string]struct {
		pages, wantFiles, wantRequests int
	}{
		"two full pages then an empty one": {pages: 2, wantFiles: 200, wantRequests: 3},
		"stops at GitHub's limit":          {pages: 1000, wantFiles: 3000, wantRequests: 30},
	} {
		test.Run(name, func(test *testing.T) {
			testServer := &server{}
			testServer.override = fullPages(testServer, testCase.pages)
			client := newTestClient(test, testServer)
			got, err := client.ListFilePatches(context.Background(), repository, 27)
			if err != nil {
				test.Fatal(err)
			}
			if len(got) != testCase.wantFiles || len(testServer.Calls()) != testCase.wantRequests {
				test.Errorf("got %d files in %d requests, want %d in %d", len(got), len(testServer.Calls()), testCase.wantFiles, testCase.wantRequests)
			}
		})
	}
}

func TestListFilePatchesNotFound(test *testing.T) {
	testServer := &server{}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"message":"Not Found"}`))
		return true
	}
	client := newTestClient(test, testServer)
	if _, err := client.ListFilePatches(context.Background(), repository, 27); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("want ErrNotFound, got %v", err)
	}
}
