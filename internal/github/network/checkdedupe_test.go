package network

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func mapChecksFromNodes(test *testing.T, nodes string, total int) github.PullRequestDetail {
	test.Helper()
	var node graphqlPullRequestDetail
	body := `{"statusCheckRollup": {"state": "SUCCESS", "contexts": {"totalCount": ` + strconv.Itoa(total) + `, "nodes": [` + nodes + `]}}}`
	if err := json.Unmarshal([]byte(body), &node); err != nil {
		test.Fatal(err)
	}
	node.Repository.NameWithOwner = "MikePors/strigidae"
	detail, err := mapDetail(node, nil, nil, nil)
	if err != nil {
		test.Fatal(err)
	}
	return detail
}

func checkRunNode(workflow, status, conclusion, startedAt, url string) string {
	started := "null"
	if startedAt != "" {
		started = `"` + startedAt + `"`
	}
	conclusionValue := "null"
	if conclusion != "" {
		conclusionValue = `"` + conclusion + `"`
	}
	return `{"__typename": "CheckRun", "name": "test", "status": "` + status + `", "conclusion": ` + conclusionValue +
		`, "detailsUrl": "` + url + `", "startedAt": ` + started + `, "checkSuite": {"workflowRun": {"workflow": {"name": "` + workflow + `"}}}}`
}

func TestMapDetailChecksKeepsLatestRun(test *testing.T) {
	const (
		early = "2026-10-01T10:00:00Z"
		late  = "2026-10-01T10:05:00Z"
	)
	tests := []struct {
		name  string
		nodes []string
		want  []github.Check
	}{
		{
			"cancelled then passing",
			[]string{checkRunNode("CI", "COMPLETED", "CANCELLED", early, "u1"), checkRunNode("CI", "COMPLETED", "SUCCESS", late, "u2")},
			[]github.Check{{Name: "CI / test", Result: github.CheckPassed, URL: "u2"}},
		},
		{
			"newest first in GitHub's order",
			[]string{checkRunNode("CI", "COMPLETED", "SUCCESS", late, "u2"), checkRunNode("CI", "COMPLETED", "CANCELLED", early, "u1")},
			[]github.Check{{Name: "CI / test", Result: github.CheckPassed, URL: "u2"}},
		},
		{
			"queued re-run after cancelled",
			[]string{checkRunNode("CI", "COMPLETED", "CANCELLED", early, "u1"), checkRunNode("CI", "QUEUED", "", "", "u2")},
			[]github.Check{{Name: "CI / test", Result: github.CheckQueued, URL: "u2"}},
		},
		{
			"queued run listed first still wins",
			[]string{checkRunNode("CI", "QUEUED", "", "", "u2"), checkRunNode("CI", "COMPLETED", "CANCELLED", early, "u1")},
			[]github.Check{{Name: "CI / test", Result: github.CheckQueued, URL: "u2"}},
		},
		{
			"tie keeps the later one",
			[]string{checkRunNode("CI", "COMPLETED", "FAILURE", early, "u1"), checkRunNode("CI", "COMPLETED", "SUCCESS", early, "u2")},
			[]github.Check{{Name: "CI / test", Result: github.CheckPassed, URL: "u2"}},
		},
		{
			"lone cancelled run stays",
			[]string{checkRunNode("CI", "COMPLETED", "CANCELLED", early, "u1")},
			[]github.Check{{Name: "CI / test", Result: github.CheckCancelled, URL: "u1"}},
		},
		{
			"same job name in different workflows",
			[]string{checkRunNode("CI", "COMPLETED", "SUCCESS", early, "u1"), checkRunNode("Docs", "COMPLETED", "SUCCESS", late, "u2")},
			[]github.Check{{Name: "CI / test", Result: github.CheckPassed, URL: "u1"}, {Name: "Docs / test", Result: github.CheckPassed, URL: "u2"}},
		},
		{
			"status contexts are untouched and survivors keep their order",
			[]string{
				`{"__typename": "StatusContext", "context": "ci/legacy", "state": "ERROR", "targetUrl": "s1"}`,
				checkRunNode("CI", "COMPLETED", "CANCELLED", early, "u1"),
				`{"__typename": "StatusContext", "context": "ci/legacy", "state": "SUCCESS", "targetUrl": "s2"}`,
				checkRunNode("CI", "COMPLETED", "SUCCESS", late, "u2"),
			},
			[]github.Check{
				{Name: "ci/legacy", Result: github.CheckFailed, URL: "s1"},
				{Name: "ci/legacy", Result: github.CheckPassed, URL: "s2"},
				{Name: "CI / test", Result: github.CheckPassed, URL: "u2"},
			},
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			detail := mapChecksFromNodes(test, strings.Join(testCase.nodes, ","), len(testCase.nodes))
			if diff := cmp.Diff(testCase.want, detail.CheckList); diff != "" {
				test.Errorf("checks (-want +got):\n%s", diff)
			}
			if detail.CheckTotal != len(testCase.want) {
				test.Errorf("CheckTotal = %d, want %d", detail.CheckTotal, len(testCase.want))
			}
		})
	}
}

func TestCheckResultGroups(test *testing.T) {
	tests := []struct {
		result github.CheckResult
		want   github.CheckGroup
	}{
		{github.CheckFailed, github.CheckGroupFailing},
		{github.CheckTimedOut, github.CheckGroupFailing},
		{github.CheckActionRequired, github.CheckGroupFailing},
		{github.CheckRunning, github.CheckGroupPending},
		{github.CheckPassed, github.CheckGroupPassing},
		{github.CheckCancelled, github.CheckGroupInactive},
		{github.CheckSkipped, github.CheckGroupInactive},
		{github.CheckNeutral, github.CheckGroupInactive},
	}
	for _, testCase := range tests {
		if got := testCase.result.Group(); got != testCase.want {
			test.Errorf("%s group = %d, want %d", testCase.result, got, testCase.want)
		}
	}
}
