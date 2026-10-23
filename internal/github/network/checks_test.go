package network

import (
	"encoding/json"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

func TestMapChecks(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n graphqlPullRequest
			if err := json.Unmarshal([]byte(`{"statusCheckRollup":`+tt.json+`}`), &n); err != nil {
				t.Fatal(err)
			}
			if got := mapChecks(n); got != tt.want {
				t.Errorf("mapChecks = %q, want %q", got, tt.want)
			}
		})
	}
}
