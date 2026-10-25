package workspace

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func (inliner *inliner) listCalls() int {
	return len(inliner.methods(fake.MethodListReviewThreads))
}

// currentResolution is a successful resolve of t-open, as the write returns it.
func (inliner *inliner) currentResolution() threadResolvedMessage {
	return threadResolvedMessage{
		origin: inliner.view.origin(), sequence: inliner.view.sequence, threadIdentifier: "t-open", resolve: true,
		location:   clientPath + ":14",
		resolution: github.ThreadResolution{Identifier: "t-open", IsResolved: true, ResolvedBy: "me"},
	}
}

// The fold is applied after a reload too: here a reload started since the
// thread was picked, so the result cannot be patched in place.
func TestFoldSurvivesTheReloadAResolutionTriggers(test *testing.T) {
	inliner := newInliner(test)
	inliner.toThread("t-resolved")
	inliner.press("z o")
	inliner.press("x")
	confirmation, _ := inliner.lastConfirm()
	inliner.run(confirmation.OnYes) // unresolve
	inliner.press("x")
	confirmation, _ = inliner.lastConfirm()
	inliner.view.sequence++ // a reload started after the thread was picked
	inliner.run(confirmation.OnYes)
	if !strings.Contains(inliner.shown(), resolvedFold) || strings.Contains(inliner.shown(), resolvedOpen) {
		test.Errorf("the thread must be folded after the reload:\n%s", inliner.shown())
	}
}

// A thread load that started before the write and lands after it holds the
// old state and must not undo the in-place update.
func TestOlderThreadLoadIsDiscardedAfterAResolution(test *testing.T) {
	inliner := newInliner(test)
	confirmation := inliner.ask("t-open")
	older := inliner.view.reloadThreads()()
	inliner.run(confirmation.OnYes)
	if !inliner.threadState("t-open").IsResolved {
		test.Fatal("setup: not resolved")
	}
	inliner.send(older)
	if !inliner.threadState("t-open").IsResolved || !strings.Contains(inliner.shown(), foldHeader) {
		test.Errorf("an older load undid the resolution:\n%s", inliner.shown())
	}
	if inliner.view.threadPending {
		test.Error("threads still pending")
	}
}

func TestThreadDeletedBetweenReadAndWriteRefreshes(test *testing.T) {
	inliner := newInliner(test)
	confirmation := inliner.ask("t-open")
	inliner.client.SetError(fake.MethodResolveReviewThread, fmt.Errorf("resolve: %w", github.ErrNotFound))
	before := inliner.listCalls()
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if !strings.Contains(inliner.lastError(), "gone") {
		test.Errorf("error = %q", inliner.lastError())
	}
	if inliner.listCalls() <= before+1 { // the re-read after the yes, then the refresh
		test.Error("the threads were not refreshed")
	}
}

func TestResultForAThreadNotShownReloads(test *testing.T) {
	inliner := newInliner(test)
	message := inliner.currentResolution()
	message.threadIdentifier = "t-unknown"
	before := inliner.listCalls()
	inliner.send(message)
	if inliner.listCalls() != before+1 {
		test.Errorf("list calls %d, want %d", inliner.listCalls(), before+1)
	}
}

func TestApplyingAResolutionRebuildsTheOverview(test *testing.T) {
	inliner := newInliner(test)
	const label = "[resolved]"
	before := strings.Count(ansi.Strip(inliner.view.overview.viewport.GetContent()), label)
	inliner.send(inliner.currentResolution())
	if after := strings.Count(ansi.Strip(inliner.view.overview.viewport.GetContent()), label); after != before+1 {
		test.Errorf("overview shows %d resolved threads, want %d", after, before+1)
	}
}
