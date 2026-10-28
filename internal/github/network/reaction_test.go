package network

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

func TestReactionMutationsSendTheCommentAndKind(test *testing.T) {
	tests := []struct {
		name      string
		remove    bool
		operation string
		field     string
	}{
		{"add", false, "AddReaction", "addReaction"},
		{"remove", true, "RemoveReaction", "removeReaction"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			testServer := &server{}
			client := newTestClient(test, testServer)
			var err error
			if testCase.remove {
				err = client.RemoveReaction(context.Background(), "PRRC_1", github.ReactionHooray)
			} else {
				err = client.AddReaction(context.Background(), "PRRC_1", github.ReactionHooray)
			}
			if err != nil {
				test.Fatal(err)
			}
			got := testServer.Calls()[0]
			if got.Operation != testCase.operation || !strings.Contains(got.Query, testCase.field+"(input:") {
				test.Errorf("op = %q, query = %q", got.Operation, got.Query)
			}
			if got.Variables["subjectId"] != "PRRC_1" || got.Variables["content"] != "HOORAY" {
				test.Errorf("unexpected variables: %v", got.Variables)
			}
		})
	}
}

// reactionCalls runs both reaction calls against a server that always answers
// with the fixture.
func reactionCalls(test *testing.T, fixture string) map[string]error {
	test.Helper()
	testServer := &server{}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		testServer.fixture(writer, http.StatusOK, fixture)
		return true
	}
	client := newTestClient(test, testServer)
	return map[string]error{
		"add":    client.AddReaction(context.Background(), "PRRC_1", github.ReactionEyes),
		"remove": client.RemoveReaction(context.Background(), "PRRC_1", github.ReactionEyes),
	}
}

func TestReactionNullPayloadIsNotFound(test *testing.T) {
	for name, err := range reactionCalls(test, "reaction_null.json") {
		if !errors.Is(err, github.ErrNotFound) {
			test.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestReactionGraphQLErrors(test *testing.T) {
	tests := []struct {
		fixture string
		want    error
	}{
		{"error_not_found.json", github.ErrNotFound},
		{"error_insufficient_scopes.json", github.ErrForbidden},
	}
	for _, testCase := range tests {
		for name, err := range reactionCalls(test, testCase.fixture) {
			if !errors.Is(err, testCase.want) {
				test.Errorf("%s with %s: err = %v, want %v", name, testCase.fixture, err, testCase.want)
			}
		}
	}
}
