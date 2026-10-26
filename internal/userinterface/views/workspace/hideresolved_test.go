package workspace

import (
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func TestHideResolvedOption(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	harness.selectFile()
	const resolvedHeader = "▸ carol 1 comment · 1d ago [resolved]"
	if !strings.Contains(harness.screen(), resolvedHeader) {
		test.Fatalf("resolved thread should show by default:\n%s", harness.screen())
	}

	harness.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: true}})
	if out := harness.screen(); strings.Contains(out, "[resolved]") || !strings.Contains(out, "▾ bob 2 comments") {
		test.Errorf("hideresolved should leave out only the resolved thread:\n%s", out)
	}
	harness.press(")")
	if strings.Contains(harness.cursorRow(), "carol") {
		test.Errorf(") should skip the hidden thread, landed on %q", harness.cursorRow())
	}

	harness.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: false}})
	if !strings.Contains(harness.screen(), resolvedHeader) {
		test.Errorf("nohideresolved should show the resolved thread again:\n%s", harness.screen())
	}
}

func TestHideResolvedThreadLeavesWhenResolved(test *testing.T) {
	inliner := newInliner(test)
	inliner.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: true}})
	confirmation := inliner.ask("t-open")
	inliner.run(confirmation.OnYes)
	if !inliner.threadState("t-open").IsResolved {
		test.Fatal("setup: the thread was not resolved")
	}
	if shown := inliner.shown(); strings.Contains(shown, "bob 2 comments") {
		test.Errorf("a thread resolved under hideresolved should leave the diff:\n%s", shown)
	}
	if threads := inliner.diffView().ThreadsAtCursor(); len(threads) != 0 {
		test.Errorf("cursor should be back on the thread's line, with nothing focused; focused %v", threads)
	}
	if cursor, ok := inliner.diffView().Cursor(); !ok || cursor.New != 14 {
		test.Errorf("cursor = %+v (ok %v), want new line 14", cursor, ok)
	}
}
