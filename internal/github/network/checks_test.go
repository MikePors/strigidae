package network

import (
	"encoding/json"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

func TestMapChecks(test *testing.T) {
	tests := []struct {
		name string
		json string
		want github.CheckState
	}{
		{"success", `{"state":"SUCCESS"}`, github.ChecksPassing},
		{"failure", `{"state":"FAILURE"}`, github.ChecksFailing},
		{"error", `{"state":"ERROR"}`, github.ChecksFailing},
		{"pending", `{"state":"PENDING"}`, github.ChecksRunning},
		{"expected", `{"state":"EXPECTED"}`, github.ChecksRunning},
		{"null rollup", `null`, github.ChecksNone},
		{"unknown state", `{"state":"WHATEVER"}`, github.ChecksNone},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var node graphqlPullRequest
			if err := json.Unmarshal([]byte(`{"statusCheckRollup":`+testCase.json+`}`), &node); err != nil {
				test.Fatal(err)
			}
			if got := mapChecks(node); got != testCase.want {
				test.Errorf("mapChecks = %q, want %q", got, testCase.want)
			}
		})
	}
}
