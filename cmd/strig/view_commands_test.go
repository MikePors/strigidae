package main

import (
	"bytes"
	"slices"
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface"
)

// The checkout's branch is the head the list preselects; with -R the checkout
// need not be the repository shown, so there is none.
func TestCurrentBranchOfTheCheckout(test *testing.T) {
	origin := "git@github.com:o/r.git"
	for _, testCase := range []struct {
		name      string
		arguments []string
		want      string
	}{
		{"inferred from the checkout", nil, "feat/current"},
		{"-R has none", []string{"-R", "o/r"}, ""},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			harness := newHarness(map[string]string{"origin": origin}, true, nil)
			var stdout, stderr bytes.Buffer
			if code := run(testCase.arguments, &stdout, &stderr, harness.dependencies); code != 0 {
				test.Fatalf("exit code = %d: %s", code, stderr.String())
			}
			if got := harness.application.currentBranch; got != testCase.want {
				test.Errorf("current branch = %q, want %q", got, testCase.want)
			}
		})
	}
}

// The shell is told the commands of every view, so ":merge" on the dashboard
// says "not available here" instead of "not a command".
func TestViewCommandsAreCollectedFromTheViews(test *testing.T) {
	got := viewCommands()
	for _, name := range []string{"merge", "approve", "copy-filename", "edit-comment", "resolve", "create", "create-draft"} {
		if !slices.Contains(got, name) {
			test.Errorf("viewCommands() lacks %q", name)
		}
	}
	// The shell's own commands win, so a view command with the same name
	// could never be reached.
	for _, name := range userinterface.CommandNames() {
		if slices.Contains(got, name) {
			test.Errorf("view command %q is also a shell command", name)
		}
	}
	seen := map[string]bool{}
	for _, name := range got {
		if seen[name] {
			test.Errorf("view command %q is declared twice", name)
		}
		seen[name] = true
	}
}
