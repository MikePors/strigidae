package workspace

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

// mergeCommand runs :merge with args.
func (composer *composer) mergeCommand(arguments string) {
	composer.t.Helper()
	command, ok := composer.v.Command("merge", arguments)
	if !ok {
		composer.t.Fatal("merge not handled")
	}
	composer.run(command)
}

func (composer *composer) callCount(method string) (count int) {
	for _, call := range composer.client.Calls() {
		if call.Method == method {
			count++
		}
	}
	return count
}

func TestResolveMethod(test *testing.T) {
	all := github.MergeStatus{Allowed: github.MergeMethods}
	squashOnly := github.MergeStatus{Allowed: []github.MergeMethod{github.MergeMethodSquash, github.MergeMethodRebase}}
	tests := []struct {
		name      string
		requested github.MergeMethod
		st        github.MergeStatus
		want      github.MergeMethod
		err       string
	}{
		{"default is merge commit when allowed", "", all, github.MergeMethodMerge, ""},
		{"default skips what the repo forbids", "", squashOnly, github.MergeMethodSquash, ""},
		{"explicit rebase", github.MergeMethodRebase, all, github.MergeMethodRebase, ""},
		{"explicit but not allowed", github.MergeMethodMerge, squashOnly, "", "merge commits are not allowed in this repo"},
		{"squash not allowed", github.MergeMethodSquash, github.MergeStatus{Allowed: []github.MergeMethod{github.MergeMethodMerge}}, "", "squash merges are not allowed in this repo"},
		{"nothing allowed", "", github.MergeStatus{}, "", "no merge method is allowed in this repo"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got, err := resolveMethod(testCase.requested, testCase.st)
			if got != testCase.want || (err == nil) != (testCase.err == "") || (err != nil && err.Error() != testCase.err) {
				test.Errorf("got %q, %v; want %q, %q", got, err, testCase.want, testCase.err)
			}
		})
	}
}

func TestMergeConfirmationAndSuccess(test *testing.T) {
	tests := []struct {
		name      string
		arguments string
		settings  fake.MergeSettings
		question  string
		method    github.MergeMethod
		info      string
	}{
		{
			"default, branch kept", "",
			fake.MergeSettings{Allowed: github.MergeMethods},
			"Merge #7 into main (merge commit)? Branch will be kept.", github.MergeMethodMerge, "Merged #7 (merge commit)",
		},
		{
			"explicit squash, branch deleted", "squash",
			fake.MergeSettings{Allowed: github.MergeMethods, DeleteBranchOnMerge: true},
			"Merge #7 into main (squash)? Branch will be deleted.", github.MergeMethodSquash, "Merged #7 (squash)",
		},
		{
			"default falls to rebase", "",
			fake.MergeSettings{Allowed: []github.MergeMethod{github.MergeMethodRebase}},
			"Merge #7 into main (rebase)? Branch will be kept.", github.MergeMethodRebase, "Merged #7 (rebase)",
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			composer := newReviewer(test)
			composer.client.SetMergeSettings(testRepository, testCase.settings)
			composer.mergeCommand(testCase.arguments)
			configuration, ok := composer.lastConfirm()
			if !ok {
				test.Fatalf("no confirmation; error: %q", composer.lastError())
			}
			if configuration.Text != testCase.question || configuration.Notice {
				test.Errorf("confirm = %+v, want text %q", configuration, testCase.question)
			}
			if len(composer.client.Merges()) != 0 {
				test.Fatal("merged before the answer")
			}
			composer.run(configuration.OnYes)
			want := []fake.Merge{{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Method: testCase.method, ExpectedHeadObjectIdentifier: headSHA}}
			if diff := cmp.Diff(want, composer.client.Merges()); diff != "" {
				test.Errorf("merges (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"Merging…", testCase.info}, composer.infos()); diff != "" {
				test.Errorf("info (-want +got):\n%s", diff)
			}
			if composer.v.detail.State != github.PullRequestStateMerged {
				test.Errorf("workspace not refreshed after success: state %v", composer.v.detail.State)
			}
			if composer.v.Loading() {
				test.Error("still loading after the merge")
			}
		})
	}
}

func TestMergeNotAllowedMethodDoesNothing(test *testing.T) {
	composer := newReviewer(test)
	composer.client.SetMergeSettings(testRepository, fake.MergeSettings{Allowed: []github.MergeMethod{github.MergeMethodMerge}})
	composer.mergeCommand("squash")
	if got := composer.lastError(); got != "squash merges are not allowed in this repo" {
		test.Errorf("error = %q", got)
	}
	if _, ok := composer.lastConfirm(); ok {
		test.Error("asked for confirmation")
	}
	if composer.callCount(fake.MethodMergePullRequest) != 0 {
		test.Error("a merge was attempted")
	}
}

func TestMergeUnknownMethodArgument(test *testing.T) {
	composer := newReviewer(test)
	before := len(composer.client.Calls())
	composer.mergeCommand("sqash \x1b[31m")
	if got := composer.lastError(); !strings.Contains(got, `unknown merge method "sqash `) || strings.Contains(got, "\x1b") {
		test.Errorf("error = %q", got)
	}
	if len(composer.client.Calls()) != before {
		test.Error("the client was called")
	}
}

func TestMergeHeadMoved(test *testing.T) {
	composer := newReviewer(test)
	composer.client.SetHeadSHA(testRepository, 7, "bbbb2222") // pushed after the view loaded
	composer.mergeCommand("")
	configuration, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no notice shown")
	}
	if !configuration.Notice || configuration.Text != headMovedText || configuration.OnYes != nil {
		test.Errorf("confirm = %+v", configuration)
	}
	composer.run(configuration.OnDismiss)
	if len(composer.client.Merges()) != 0 || composer.callCount(fake.MethodMergePullRequest) != 0 {
		test.Error("merged although the head moved")
	}
	if composer.v.detail.HeadSHA != "bbbb2222" {
		test.Errorf("workspace head = %q, want it refreshed", composer.v.detail.HeadSHA)
	}
}

func TestMergeHeadMovesAfterConfirmationIsRefusedAndNotRetried(test *testing.T) {
	composer := newReviewer(test)
	composer.mergeCommand("")
	configuration, _ := composer.lastConfirm()
	composer.client.SetHeadSHA(testRepository, 7, "bbbb2222") // pushed while the question was open
	composer.run(configuration.OnYes)
	if composer.callCount(fake.MethodMergePullRequest) != 1 || len(composer.client.Merges()) != 0 {
		test.Errorf("merge calls = %d, merges = %d", composer.callCount(fake.MethodMergePullRequest), len(composer.client.Merges()))
	}
	if composer.lastError() == "" {
		test.Error("no error shown")
	}
	if composer.v.detail.HeadSHA != "bbbb2222" {
		test.Error("workspace not refreshed")
	}
}

func TestMergeRefusalShownThenRefreshed(test *testing.T) {
	composer := newReviewer(test)
	composer.mergeCommand("")
	configuration, _ := composer.lastConfirm()
	loads := composer.callCount(fake.MethodGetPullRequest)
	composer.client.SetError(fake.MethodMergePullRequest, errors.New("2 of 3 required status checks are expected\x1b[31m"))
	composer.run(configuration.OnYes)

	got := composer.lastError()
	if !strings.Contains(got, "2 of 3 required status checks are expected") || strings.Contains(got, "\x1b") {
		test.Errorf("error = %q", got)
	}
	if composer.callCount(fake.MethodMergePullRequest) != 1 {
		test.Errorf("merge attempted %d times, want 1 (never retried)", composer.callCount(fake.MethodMergePullRequest))
	}
	if composer.callCount(fake.MethodGetPullRequest) <= loads {
		test.Error("workspace not refreshed after the refusal")
	}
	if composer.v.merge.merging || composer.v.Loading() {
		test.Error("still merging after the refusal")
	}
	if len(composer.client.Merges()) != 0 {
		test.Error("a refused merge was recorded")
	}
}

func TestMergeBlockedWhileInFlight(test *testing.T) {
	composer := newReviewer(test)
	composer.mergeCommand("")
	configuration, _ := composer.lastConfirm()
	// Deliver the confirmation's message without running the merge command.
	send := configuration.OnYes()
	view, _ := composer.v.Update(send)
	composer.v = view.(*View)
	if !composer.v.merge.merging || !composer.v.Loading() {
		test.Fatalf("merging = %v, loading = %v", composer.v.merge.merging, composer.v.Loading())
	}
	before := len(composer.client.Calls())
	composer.messages = nil
	composer.mergeCommand("")
	if diff := cmp.Diff([]string{"a merge is in progress"}, composer.infos()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if len(composer.client.Calls()) != before {
		test.Error("a second merge started querying")
	}
}

func TestMergeNeedsLoadedPullRequestAndDiff(test *testing.T) {
	test.Run("not loaded", func(test *testing.T) {
		testComposer := &composer{harness: newHarness(test, 110, 40)} // Init not delivered
		testComposer.mergeCommand("")
		if diff := cmp.Diff([]string{"pull request not loaded yet"}, testComposer.infos()); diff != "" {
			test.Errorf("info (-want +got):\n%s", diff)
		}
	})
	test.Run("diff not loaded", func(test *testing.T) {
		composer := newReviewer(test)
		composer.v.diff = DiffState{}
		composer.mergeCommand("")
		if diff := cmp.Diff([]string{"diff not loaded"}, composer.infos()); diff != "" {
			test.Errorf("info (-want +got):\n%s", diff)
		}
	})
}

func TestMergeFlowIgnoresOtherFlowsAndStaleResults(test *testing.T) {
	var flow, other mergeFlow
	environment := flowEnvironment(fake.New())
	if _, handled := flow.update(refreshMessage{}, environment); handled {
		test.Error("an unrelated message was claimed")
	}
	command, handled := flow.update(mergedMessage{flow: &other, repository: testRepository, number: 7}, environment)
	if !handled || command != nil {
		test.Errorf("foreign result: handled=%v cmd=%v", handled, command != nil)
	}
	flow.sequence = 2
	command, handled = flow.update(mergeFreshMessage{flow: &flow, repository: testRepository, number: 7, sequence: 1}, environment)
	if !handled || command != nil {
		test.Errorf("stale result: handled=%v cmd=%v", handled, command != nil)
	}
}

func TestMergeQuestionCleansBaseBranch(test *testing.T) {
	query := mergeQuestion(7, github.MergeMethodMerge, github.MergeStatus{BaseBranch: "ma\x1b[31min"})
	if strings.Contains(query, "\x1b") {
		test.Errorf("question = %q", query)
	}
}
