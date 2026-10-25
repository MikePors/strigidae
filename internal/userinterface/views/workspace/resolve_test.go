package workspace

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
)

// Threads in testThreads on internal/sync/client.go: t-open (line 14, shown
// open) and t-resolved (line 12, folded). A folded thread is one line led by
// the first glyph, an open one by the second.
const (
	foldedGlyph  = "▸"
	openGlyph    = "▾"
	openHeader   = openGlyph + " bob"
	foldHeader   = foldedGlyph + " bob"
	resolvedOpen = openGlyph + " carol"
	resolvedFold = foldedGlyph + " carol"
)

func (inliner *inliner) shown() string { return ansi.Strip(inliner.render()) }

func (inliner *inliner) threadState(identifier string) github.ReviewThread {
	inliner.test.Helper()
	for _, thread := range inliner.view.threads {
		if thread.Identifier == identifier {
			return thread
		}
	}
	inliner.test.Fatalf("thread %s not shown", identifier)
	return github.ReviewThread{}
}

func (inliner *inliner) resolveCalls() []fake.Call {
	var out []fake.Call
	for _, call := range inliner.client.Calls() {
		if call.Method == fake.MethodResolveReviewThread || call.Method == fake.MethodUnresolveReviewThread {
			out = append(out, call)
		}
	}
	return out
}

// ask presses x and returns the question asked.
func (inliner *inliner) ask(identifier string) userinterface.ConfirmMessage {
	inliner.test.Helper()
	inliner.toThread(identifier)
	inliner.messages = nil
	inliner.press("x")
	confirmation, ok := inliner.lastConfirm()
	if !ok {
		inliner.test.Fatalf("no question asked; messages %v", inliner.messages)
	}
	return confirmation
}

func TestResolveAsksThenWritesOnce(test *testing.T) {
	inliner := newInliner(test)
	if !strings.Contains(inliner.shown(), openHeader) {
		test.Fatal("setup: the open thread is not shown open")
	}
	confirmation := inliner.ask("t-open")
	if want := "Resolve thread on internal/sync/client.go:14?"; confirmation.Text != want {
		test.Errorf("question = %q, want %q", confirmation.Text, want)
	}
	if len(inliner.resolveCalls()) != 0 {
		test.Fatal("written before the answer")
	}
	if inliner.view.writes != 0 || inliner.view.CapturesKeys() {
		test.Errorf("a question left the view busy: writes %d", inliner.view.writes)
	}
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	want := []fake.Call{{Method: fake.MethodResolveReviewThread, ThreadIdentifier: "t-open"}}
	if diff := cmp.Diff(want, inliner.resolveCalls()); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Resolved thread on internal/sync/client.go:14"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if thread := inliner.threadState("t-open"); !thread.IsResolved {
		test.Errorf("thread not updated in place: %+v", thread)
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight: %d", inliner.view.writes)
	}
	shown := inliner.shown()
	if !strings.Contains(shown, foldHeader) || strings.Contains(shown, openHeader) {
		test.Errorf("a resolved thread must fold closed with its label:\n%s", shown)
	}
}

func TestUnresolveAsksThenWritesOnce(test *testing.T) {
	inliner := newInliner(test)
	if !strings.Contains(inliner.shown(), resolvedFold) {
		test.Fatal("setup: the resolved thread starts open")
	}
	confirmation := inliner.ask("t-resolved")
	if want := "Unresolve thread on internal/sync/client.go:12?"; confirmation.Text != want {
		test.Errorf("question = %q, want %q", confirmation.Text, want)
	}
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	want := []fake.Call{{Method: fake.MethodUnresolveReviewThread, ThreadIdentifier: "t-resolved"}}
	if diff := cmp.Diff(want, inliner.resolveCalls()); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Unresolved thread on internal/sync/client.go:12"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if thread := inliner.threadState("t-resolved"); thread.IsResolved {
		test.Errorf("thread not updated in place: %+v", thread)
	}
	if !strings.Contains(inliner.shown(), resolvedOpen) {
		test.Errorf("an unresolved thread must open:\n%s", inliner.shown())
	}
}

// A thread the user opened by hand folds closed anyway once it is resolved.
func TestResolveFoldsAThreadTheUserOpened(test *testing.T) {
	inliner := newInliner(test)
	inliner.toThread("t-resolved")
	inliner.press("z o")
	if !strings.Contains(inliner.shown(), resolvedOpen) {
		test.Fatal("setup: z o did not open the thread")
	}
	inliner.press("x")
	confirmation, _ := inliner.lastConfirm()
	inliner.run(confirmation.OnYes) // unresolve
	inliner.press("x")
	confirmation, _ = inliner.lastConfirm()
	inliner.run(confirmation.OnYes) // resolve again
	if !strings.Contains(inliner.shown(), resolvedFold) {
		test.Errorf("resolving must fold the thread:\n%s", inliner.shown())
	}
}

func TestDecliningWritesNothing(test *testing.T) {
	inliner := newInliner(test)
	inliner.ask("t-open") // declining is the shell dropping OnYes
	if len(inliner.resolveCalls()) != 0 || inliner.threadState("t-open").IsResolved {
		test.Error("something was written")
	}
	inliner.press("x") // the view is free to ask again
	if _, ok := inliner.lastConfirm(); !ok {
		test.Error("the view stayed busy after a declined question")
	}
}

func TestResolvedByAnotherBeforeAskingRefusesAndRefreshes(test *testing.T) {
	inliner := newInliner(test)
	inliner.toThread("t-open")
	if _, err := inliner.client.ResolveReviewThread(test.Context(), "t-open"); err != nil { // on github.com
		test.Fatal(err)
	}
	inliner.messages = nil
	inliner.press("x")
	if _, asked := inliner.lastConfirm(); asked {
		test.Error("asked about a state that no longer holds")
	}
	if !strings.Contains(inliner.lastError(), "already resolved") {
		test.Errorf("error = %q", inliner.lastError())
	}
	if diff := cmp.Diff([]string{fake.MethodResolveReviewThread}, callMethods(inliner.resolveCalls())); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}
	if !inliner.threadState("t-open").IsResolved {
		test.Error("the view did not refresh")
	}
}

func TestChangedBetweenAskingAndYesWritesNothing(test *testing.T) {
	inliner := newInliner(test)
	confirmation := inliner.ask("t-open")
	if _, err := inliner.client.ResolveReviewThread(test.Context(), "t-open"); err != nil { // while the question is open
		test.Fatal(err)
	}
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if diff := cmp.Diff([]string{fake.MethodResolveReviewThread}, callMethods(inliner.resolveCalls())); diff != "" {
		test.Errorf("only the other writer's call may exist (-want +got):\n%s", diff)
	}
	if !strings.Contains(inliner.lastError(), "already resolved") {
		test.Errorf("error = %q", inliner.lastError())
	}
	if !inliner.threadState("t-open").IsResolved || inliner.view.writes != 0 {
		test.Error("the view did not refresh or stayed busy")
	}
}

func TestThreadGoneBeforeYes(test *testing.T) {
	inliner := newInliner(test)
	confirmation := inliner.ask("t-open")
	inliner.client.SetReviewThreads(testRepository, 7, nil)
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if len(inliner.resolveCalls()) != 0 || !strings.Contains(inliner.lastError(), "gone") {
		test.Errorf("calls %v, error %q", inliner.resolveCalls(), inliner.lastError())
	}
	if len(inliner.view.threads) != 0 {
		test.Error("the view did not refresh")
	}
}

func TestResolveFailureShowsErrorAndChangesNothing(test *testing.T) {
	inliner := newInliner(test)
	confirmation := inliner.ask("t-open")
	inliner.client.SetError(fake.MethodResolveReviewThread, errors.New("boom \x1b[31m"))
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	err := inliner.lastError()
	if !strings.Contains(err, "boom") || strings.Contains(err, "\x1b") {
		test.Errorf("error = %q", err)
	}
	if inliner.threadState("t-open").IsResolved || !strings.Contains(inliner.shown(), openHeader) {
		test.Error("a failed write changed the thread")
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight: %d", inliner.view.writes)
	}
}

func TestResolveWithSeveralThreadsOrNone(test *testing.T) {
	inliner := newInliner(test)
	two := []github.ReviewThread{
		{Identifier: "a", Path: clientPath, Line: 14, DiffSide: github.SideRight, Comments: []github.ReviewComment{newComment("bob", "one", time.Hour)}},
		{Identifier: "b", Path: clientPath, Line: 14, DiffSide: github.SideRight, Comments: []github.ReviewComment{newComment("eve", "two", time.Hour)}},
	}
	inliner.client.SetReviewThreads(testRepository, 7, two)
	inliner.run(inliner.view.load())

	inliner.press("g g") // a hunk header: no thread
	inliner.messages = nil
	inliner.press("x")
	if diff := cmp.Diff([]string{"no comment thread here"}, inliner.infoMessages()); diff != "" {
		test.Errorf("none (-want +got):\n%s", diff)
	}

	inliner.cursorTo("two threads", func(diffView *diffview.Model) bool {
		return len(diffView.ThreadsAtCursor()) == 2
	})
	inliner.messages = nil
	inliner.press("x")
	if _, asked := inliner.lastConfirm(); asked {
		test.Error("asked with several threads on the line")
	}
	if diff := cmp.Diff([]string{"several threads on this line: move onto one to resolve it"}, inliner.infoMessages()); diff != "" {
		test.Errorf("several (-want +got):\n%s", diff)
	}
	if len(inliner.resolveCalls()) != 0 {
		test.Error("something was written")
	}
}

func TestResolveCommands(test *testing.T) {
	tests := []struct {
		name       string
		command    string
		thread     string
		wantInfo   string
		wantAsk    string
		wantMethod string
	}{
		{"resolve an open thread", "resolve", "t-open", "", "Resolve thread on internal/sync/client.go:14?", fake.MethodResolveReviewThread},
		{"resolve a resolved thread", "resolve", "t-resolved", "thread is already resolved", "", ""},
		{"unresolve a resolved thread", "unresolve", "t-resolved", "", "Unresolve thread on internal/sync/client.go:12?", fake.MethodUnresolveReviewThread},
		{"unresolve an open thread", "unresolve", "t-open", "thread is not resolved", "", ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.toThread(testCase.thread)
			inliner.messages = nil
			command, handled := inliner.view.Command(testCase.command, "")
			if !handled {
				test.Fatal("command not handled")
			}
			inliner.run(command)
			confirmation, asked := inliner.lastConfirm()
			if testCase.wantAsk == "" {
				if asked {
					test.Errorf("asked %q", confirmation.Text)
				}
				if diff := cmp.Diff([]string{testCase.wantInfo}, inliner.infoMessages()); diff != "" {
					test.Errorf("info (-want +got):\n%s", diff)
				}
				return
			}
			if !asked || confirmation.Text != testCase.wantAsk {
				test.Fatalf("question = %q (%v), want %q", confirmation.Text, asked, testCase.wantAsk)
			}
			if len(inliner.resolveCalls()) != 0 {
				test.Fatal("written before the answer")
			}
			inliner.run(confirmation.OnYes)
			want := []string{testCase.wantMethod}
			if diff := cmp.Diff(want, callMethods(inliner.resolveCalls())); diff != "" {
				test.Errorf("calls (-want +got):\n%s", diff)
			}
		})
	}
}

func callMethods(calls []fake.Call) []string {
	var out []string
	for _, call := range calls {
		out = append(out, call.Method)
	}
	return out
}

func TestStaleResolveResultsAreIgnored(test *testing.T) {
	inliner := newInliner(test)
	good := threadResolvedMessage{
		origin: inliner.view.origin(), sequence: inliner.view.sequence, threadIdentifier: "t-open", resolve: true,
		location:   clientPath + ":14",
		resolution: github.ThreadResolution{Identifier: "t-open", IsResolved: true, ResolvedBy: "me"},
	}
	otherPullRequest := good
	otherPullRequest.origin.number = 8
	otherView := good
	otherView.origin.owner = New(inliner.client, testRepository, 7)
	for name, message := range map[string]threadResolvedMessage{"another pull request": otherPullRequest, "another workspace": otherView} {
		inliner.messages = nil
		inliner.send(message)
		if inliner.threadState("t-open").IsResolved || len(inliner.messages) != 0 {
			test.Errorf("%s: result applied (messages %v)", name, inliner.messages)
		}
	}

	// A reload since the thread was picked: the thread is read again, not patched.
	older := good
	older.sequence = inliner.view.sequence - 1
	inliner.messages = nil
	inliner.send(older)
	if inliner.threadState("t-open").IsResolved {
		test.Error("a result from before a reload patched the thread")
	}

	inliner.send(good)
	if !inliner.threadState("t-open").IsResolved || inliner.threadState("t-open").ResolvedBy != "me" {
		test.Errorf("the current result was not applied: %+v", inliner.threadState("t-open"))
	}
}

func TestResolveKeyIsInHelp(test *testing.T) {
	view := loaded(test, 100, 30).view
	var found bool
	for _, group := range view.FullHelp() {
		for _, binding := range group {
			if keys := binding.Keys(); len(keys) == 1 && keys[0] == "x" {
				found = true
			}
		}
	}
	if !found {
		test.Error("x is not in the help")
	}
}
