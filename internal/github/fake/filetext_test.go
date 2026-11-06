package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestFileText(test *testing.T) {
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	client := New()
	client.SetFileText(repository, "abc", "main.go", "package main\n")
	client.SetFileText(repository, "def", "main.go", "package other\n")
	client.SetFileText(repository, "abc", "empty.txt", "")
	client.SetFileTextUnavailable(repository, "abc", "logo.png")

	for _, testCase := range []struct {
		name       string
		repository github.Repository
		commit     string
		path       string
		wantText   string
		wantErr    error
	}{
		{"text", repository, "abc", "main.go", "package main\n", nil},
		{"same path at another commit", repository, "def", "main.go", "package other\n", nil},
		{"empty file", repository, "abc", "empty.txt", "", nil},
		{"unknown path", repository, "abc", "missing.go", "", github.ErrNotFound},
		{"unknown commit", repository, "zzz", "main.go", "", github.ErrNotFound},
		{"other repository", github.Repository{Owner: "x", Name: "y"}, "abc", "main.go", "", github.ErrNotFound},
		{"unavailable text", repository, "abc", "logo.png", "", github.ErrNoFileText},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			text, err := client.FileText(context.Background(), testCase.repository, testCase.commit, testCase.path)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
			if text != testCase.wantText {
				test.Errorf("text = %q, want %q", text, testCase.wantText)
			}
		})
	}
}

func TestFileTextRecordsCalls(test *testing.T) {
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	client := New()
	_, _ = client.FileText(context.Background(), repository, "abc", "main.go")
	want := []Call{{Method: MethodFileText, Repository: repository, CommitObjectIdentifier: "abc", Path: "main.go"}}
	if diff := cmp.Diff(want, client.Calls()); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}
}

func TestFileTextInjectedError(test *testing.T) {
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	client := New()
	client.SetFileText(repository, "abc", "main.go", "package main\n")
	injected := errors.New("boom")
	client.SetError(MethodFileText, injected)
	if _, err := client.FileText(context.Background(), repository, "abc", "main.go"); !errors.Is(err, injected) {
		test.Errorf("err = %v, want the injected error", err)
	}
	client.SetError(MethodFileText, nil)
	if _, err := client.FileText(context.Background(), repository, "abc", "main.go"); err != nil {
		test.Errorf("after clearing: err = %v", err)
	}
}
