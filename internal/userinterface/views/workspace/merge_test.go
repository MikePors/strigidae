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
func (c *composer) mergeCommand(args string) {
	c.t.Helper()
	cmd, ok := c.v.Command("merge", args)
	if !ok {
		c.t.Fatal("merge not handled")
	}
	c.run(cmd)
}

func (c *composer) callCount(method string) (n int) {
	for _, call := range c.client.Calls() {
		if call.Method == method {
			n++
		}
	}
	return n
}

func TestResolveMethod(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMethod(tt.requested, tt.st)
			if got != tt.want || (err == nil) != (tt.err == "") || (err != nil && err.Error() != tt.err) {
				t.Errorf("got %q, %v; want %q, %q", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestMergeConfirmationAndSuccess(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newReviewer(t)
			c.client.SetMergeSettings(testRepository, tt.settings)
			c.mergeCommand(tt.arguments)
			conf, ok := c.lastConfirm()
			if !ok {
				t.Fatalf("no confirmation; error: %q", c.lastError())
			}
			if conf.Text != tt.question || conf.Notice {
				t.Errorf("confirm = %+v, want text %q", conf, tt.question)
			}
			if len(c.client.Merges()) != 0 {
				t.Fatal("merged before the answer")
			}
			c.run(conf.OnYes)
			want := []fake.Merge{{Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Method: tt.method, ExpectedHeadObjectIdentifier: headSHA}}
			if diff := cmp.Diff(want, c.client.Merges()); diff != "" {
				t.Errorf("merges (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"Merging…", tt.info}, c.infos()); diff != "" {
				t.Errorf("info (-want +got):\n%s", diff)
			}
			if c.v.detail.State != github.PullRequestStateMerged {
				t.Errorf("workspace not refreshed after success: state %v", c.v.detail.State)
			}
			if c.v.Loading() {
				t.Error("still loading after the merge")
			}
		})
	}
}

func TestMergeNotAllowedMethodDoesNothing(t *testing.T) {
	c := newReviewer(t)
	c.client.SetMergeSettings(testRepository, fake.MergeSettings{Allowed: []github.MergeMethod{github.MergeMethodMerge}})
	c.mergeCommand("squash")
	if got := c.lastError(); got != "squash merges are not allowed in this repo" {
		t.Errorf("error = %q", got)
	}
	if _, ok := c.lastConfirm(); ok {
		t.Error("asked for confirmation")
	}
	if c.callCount(fake.MethodMergePullRequest) != 0 {
		t.Error("a merge was attempted")
	}
}

func TestMergeUnknownMethodArgument(t *testing.T) {
	c := newReviewer(t)
	before := len(c.client.Calls())
	c.mergeCommand("sqash \x1b[31m")
	if got := c.lastError(); !strings.Contains(got, `unknown merge method "sqash `) || strings.Contains(got, "\x1b") {
		t.Errorf("error = %q", got)
	}
	if len(c.client.Calls()) != before {
		t.Error("the client was called")
	}
}

func TestMergeHeadMoved(t *testing.T) {
	c := newReviewer(t)
	c.client.SetHeadSHA(testRepository, 7, "bbbb2222") // pushed after the view loaded
	c.mergeCommand("")
	conf, ok := c.lastConfirm()
	if !ok {
		t.Fatal("no notice shown")
	}
	if !conf.Notice || conf.Text != headMovedText || conf.OnYes != nil {
		t.Errorf("confirm = %+v", conf)
	}
	c.run(conf.OnDismiss)
	if len(c.client.Merges()) != 0 || c.callCount(fake.MethodMergePullRequest) != 0 {
		t.Error("merged although the head moved")
	}
	if c.v.detail.HeadSHA != "bbbb2222" {
		t.Errorf("workspace head = %q, want it refreshed", c.v.detail.HeadSHA)
	}
}

func TestMergeHeadMovesAfterConfirmationIsRefusedAndNotRetried(t *testing.T) {
	c := newReviewer(t)
	c.mergeCommand("")
	conf, _ := c.lastConfirm()
	c.client.SetHeadSHA(testRepository, 7, "bbbb2222") // pushed while the question was open
	c.run(conf.OnYes)
	if c.callCount(fake.MethodMergePullRequest) != 1 || len(c.client.Merges()) != 0 {
		t.Errorf("merge calls = %d, merges = %d", c.callCount(fake.MethodMergePullRequest), len(c.client.Merges()))
	}
	if c.lastError() == "" {
		t.Error("no error shown")
	}
	if c.v.detail.HeadSHA != "bbbb2222" {
		t.Error("workspace not refreshed")
	}
}

func TestMergeRefusalShownThenRefreshed(t *testing.T) {
	c := newReviewer(t)
	c.mergeCommand("")
	conf, _ := c.lastConfirm()
	loads := c.callCount(fake.MethodGetPullRequest)
	c.client.SetError(fake.MethodMergePullRequest, errors.New("2 of 3 required status checks are expected\x1b[31m"))
	c.run(conf.OnYes)

	got := c.lastError()
	if !strings.Contains(got, "2 of 3 required status checks are expected") || strings.Contains(got, "\x1b") {
		t.Errorf("error = %q", got)
	}
	if c.callCount(fake.MethodMergePullRequest) != 1 {
		t.Errorf("merge attempted %d times, want 1 (never retried)", c.callCount(fake.MethodMergePullRequest))
	}
	if c.callCount(fake.MethodGetPullRequest) <= loads {
		t.Error("workspace not refreshed after the refusal")
	}
	if c.v.merge.merging || c.v.Loading() {
		t.Error("still merging after the refusal")
	}
	if len(c.client.Merges()) != 0 {
		t.Error("a refused merge was recorded")
	}
}

func TestMergeBlockedWhileInFlight(t *testing.T) {
	c := newReviewer(t)
	c.mergeCommand("")
	conf, _ := c.lastConfirm()
	// Deliver the confirmation's message without running the merge command.
	send := conf.OnYes()
	v, _ := c.v.Update(send)
	c.v = v.(*View)
	if !c.v.merge.merging || !c.v.Loading() {
		t.Fatalf("merging = %v, loading = %v", c.v.merge.merging, c.v.Loading())
	}
	before := len(c.client.Calls())
	c.messages = nil
	c.mergeCommand("")
	if diff := cmp.Diff([]string{"a merge is in progress"}, c.infos()); diff != "" {
		t.Errorf("info (-want +got):\n%s", diff)
	}
	if len(c.client.Calls()) != before {
		t.Error("a second merge started querying")
	}
}

func TestMergeNeedsLoadedPullRequestAndDiff(t *testing.T) {
	t.Run("not loaded", func(t *testing.T) {
		c := &composer{harness: newHarness(t, 110, 40)} // Init not delivered
		c.mergeCommand("")
		if diff := cmp.Diff([]string{"pull request not loaded yet"}, c.infos()); diff != "" {
			t.Errorf("info (-want +got):\n%s", diff)
		}
	})
	t.Run("diff not loaded", func(t *testing.T) {
		c := newReviewer(t)
		c.v.diff = DiffState{}
		c.mergeCommand("")
		if diff := cmp.Diff([]string{"diff not loaded"}, c.infos()); diff != "" {
			t.Errorf("info (-want +got):\n%s", diff)
		}
	})
}

func TestMergeFlowIgnoresOtherFlowsAndStaleResults(t *testing.T) {
	var f, other mergeFlow
	env := flowEnvironment(fake.New())
	if _, handled := f.update(refreshMessage{}, env); handled {
		t.Error("an unrelated message was claimed")
	}
	cmd, handled := f.update(mergedMessage{flow: &other, repository: testRepository, number: 7}, env)
	if !handled || cmd != nil {
		t.Errorf("foreign result: handled=%v cmd=%v", handled, cmd != nil)
	}
	f.sequence = 2
	cmd, handled = f.update(mergeFreshMessage{flow: &f, repository: testRepository, number: 7, sequence: 1}, env)
	if !handled || cmd != nil {
		t.Errorf("stale result: handled=%v cmd=%v", handled, cmd != nil)
	}
}

func TestMergeQuestionCleansBaseBranch(t *testing.T) {
	q := mergeQuestion(7, github.MergeMethodMerge, github.MergeStatus{BaseBranch: "ma\x1b[31min"})
	if strings.Contains(q, "\x1b") {
		t.Errorf("question = %q", q)
	}
}
