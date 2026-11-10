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

func blobTextsServer(fixture string) *server {
	testServer := &server{}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		testServer.fixture(writer, http.StatusOK, fixture)
		return true
	}
	return testServer
}

func TestBlobTexts(test *testing.T) {
	testServer := blobTextsServer("BlobTexts.json")
	texts, err := newTestClient(test, testServer).BlobTexts(context.Background(), repository, []string{"fe7e06ff70", "1234567"})
	if err != nil {
		test.Fatal(err)
	}
	want := []github.BlobText{{Text: "package main\n/* a\n b */\n"}, {Text: ""}}
	if diff := cmp.Diff(want, texts); diff != "" {
		test.Errorf("texts (-want +got):\n%s", diff)
	}
	calls := testServer.Calls()
	if len(calls) != 1 || calls[0].Operation != operationBlobTexts {
		test.Fatalf("calls = %+v, want one %s query", calls, operationBlobTexts)
	}
	wantVariables := map[string]any{"owner": "MikePors", "name": "strigidae", "expression0": "fe7e06ff70", "expression1": "1234567"}
	if diff := cmp.Diff(wantVariables, calls[0].Variables); diff != "" {
		test.Errorf("variables (-want +got):\n%s", diff)
	}
	if strings.Contains(calls[0].Query, "fe7e06ff70") {
		test.Error("an identifier was spliced into the query text")
	}
}

func TestBlobTextsUnusableEntries(test *testing.T) {
	identifiers := []string{"1111111", "2222222", "3333333", "4444444", "5555555", "abcdef0"}
	texts, err := newTestClient(test, blobTextsServer("BlobTexts.unusable.json")).BlobTexts(context.Background(), repository, identifiers)
	if err != nil {
		test.Fatal(err)
	}
	wantErrors := []error{github.ErrNotFound, github.ErrNotFound, github.ErrNoFileText, github.ErrNoFileText, github.ErrNotFound, nil}
	if len(texts) != len(wantErrors) {
		test.Fatalf("got %d entries, want %d", len(texts), len(wantErrors))
	}
	for i, want := range wantErrors {
		if !errors.Is(texts[i].Err, want) && (want != nil || texts[i].Err != nil) {
			test.Errorf("entry %d err = %v, want %v", i, texts[i].Err, want)
		}
		if texts[i].Err != nil {
			if texts[i].Text != "" {
				test.Errorf("entry %d has text and an error", i)
			}
			for _, leaked := range []string{"binary bytes", "very large file", "a ref named like the id"} {
				if strings.Contains(texts[i].Err.Error(), leaked) {
					test.Errorf("entry %d error %q carries file contents", i, texts[i].Err)
				}
			}
		}
	}
	if texts[5].Text != "upper case oid" {
		test.Errorf("case-insensitive match text = %q", texts[5].Text)
	}
}

func TestBlobTextsNoIdentifiersAsksNothing(test *testing.T) {
	testServer := &server{}
	texts, err := newTestClient(test, testServer).BlobTexts(context.Background(), repository, nil)
	if err != nil || texts != nil || len(testServer.Calls()) != 0 {
		test.Errorf("texts, err, calls = %v, %v, %v; want none", texts, err, testServer.Calls())
	}
}

func TestBlobTextsUnknownRepository(test *testing.T) {
	testServer := &server{override: func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"repository":null}}`))
		return true
	}}
	_, err := newTestClient(test, testServer).BlobTexts(context.Background(), repository, []string{"1111111"})
	if !errors.Is(err, github.ErrNotFound) {
		test.Errorf("err = %v, want ErrNotFound", err)
	}
}
