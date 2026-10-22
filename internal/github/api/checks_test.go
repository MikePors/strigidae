package api

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
		{"success", `{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}`, github.ChecksPassing},
		{"failure", `{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}`, github.ChecksFailing},
		{"error", `{"nodes":[{"commit":{"statusCheckRollup":{"state":"ERROR"}}}]}`, github.ChecksFailing},
		{"pending", `{"nodes":[{"commit":{"statusCheckRollup":{"state":"PENDING"}}}]}`, github.ChecksRunning},
		{"expected", `{"nodes":[{"commit":{"statusCheckRollup":{"state":"EXPECTED"}}}]}`, github.ChecksRunning},
		{"null rollup", `{"nodes":[{"commit":{"statusCheckRollup":null}}]}`, github.ChecksNone},
		{"no commits", `{"nodes":[]}`, github.ChecksNone},
		{"unknown state", `{"nodes":[{"commit":{"statusCheckRollup":{"state":"WHATEVER"}}}]}`, github.ChecksNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n gqlPullRequest
			if err := json.Unmarshal([]byte(`{"commits":`+tt.json+`}`), &n); err != nil {
				t.Fatal(err)
			}
			if got := mapChecks(n); got != tt.want {
				t.Errorf("mapChecks = %q, want %q", got, tt.want)
			}
		})
	}
}
