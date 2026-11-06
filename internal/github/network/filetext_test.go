package network

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestFileText(test *testing.T) {
	commit := sha(7)
	for _, testCase := range []struct {
		name     string
		fixture  string
		wantText string
		wantErr  error
		// fixtureText is text the fixture returns that must not leak into an error.
		fixtureText string
	}{
		{"text file", "FileText.json", "package main\n\n/* a comment\n   over lines */\nfunc main() {}\n", nil, ""},
		{"empty file", "FileText.empty.json", "", nil, ""},
		{"missing path or commit", "FileText.missing.json", "", github.ErrNotFound, ""},
		{"directory", "FileText.tree.json", "", github.ErrNotFound, ""},
		{"binary file", "FileText.binary.json", "", github.ErrNoFileText, ""},
		{"binary file with text", "FileText.binarywithtext.json", "", github.ErrNoFileText, "binary bytes"},
		{"truncated file", "FileText.truncated.json", "", github.ErrNoFileText, "very large file"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			testServer := &server{}
			testServer.override = func(_ call, writer http.ResponseWriter) bool {
				testServer.fixture(writer, http.StatusOK, testCase.fixture)
				return true
			}
			text, err := newTestClient(test, testServer).FileText(context.Background(), repository, commit, "cmd/strig/main.go")
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
			if diff := cmp.Diff(testCase.wantText, text); diff != "" {
				test.Errorf("text (-want +got):\n%s", diff)
			}
			if testCase.fixtureText != "" && strings.Contains(err.Error(), testCase.fixtureText) {
				test.Errorf("error %q carries file contents", err)
			}
			calls := testServer.Calls()
			if len(calls) != 1 || calls[0].Operation != operationFileText {
				test.Fatalf("calls = %+v, want one %s query", calls, operationFileText)
			}
			wantVariables := map[string]any{"owner": "MikePors", "name": "strigidae", "expression": commit + ":cmd/strig/main.go"}
			if diff := cmp.Diff(wantVariables, calls[0].Variables); diff != "" {
				test.Errorf("variables (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileTextUnknownRepository(test *testing.T) {
	testServer := &server{override: func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"repository":null}}`))
		return true
	}}
	_, err := newTestClient(test, testServer).FileText(context.Background(), repository, sha(7), "README.md")
	if !errors.Is(err, github.ErrNotFound) {
		test.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestFileTextGraphQLNotFound(test *testing.T) {
	testServer := &server{}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		testServer.fixture(writer, http.StatusOK, "error_not_found.json")
		return true
	}
	_, err := newTestClient(test, testServer).FileText(context.Background(), repository, sha(7), "README.md")
	if !errors.Is(err, github.ErrNotFound) {
		test.Errorf("err = %v, want ErrNotFound", err)
	}
}
