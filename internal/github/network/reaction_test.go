package network

import (
	"context"
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
