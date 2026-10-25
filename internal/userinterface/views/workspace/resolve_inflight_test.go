package workspace

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// replyElsewhere adds a comment to the resolved thread on the server.
func (inliner *inliner) replyElsewhere() {
	inliner.test.Helper()
	if _, err := inliner.client.ReplyToThread(inliner.test.Context(), "t-resolved", "late reply"); err != nil {
		inliner.test.Fatal(err)
	}
}

func (inliner *inliner) resolvedThreadComments() int {
	return len(inliner.threadState("t-resolved").Comments)
}

// A thread load in flight (a reply's reload, or R) may carry newer data than
// the screen, so a resolution must not drop it.
func TestResolveWithAThreadLoadInFlightKeepsItsData(test *testing.T) {
	tests := []struct {
		name  string
		start func(inliner *inliner) tea.Cmd
	}{
		{"a reply's reload", func(inliner *inliner) tea.Cmd { return inliner.view.reloadThreads() }},
		{"R", func(inliner *inliner) tea.Cmd { return inliner.view.load() }},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			confirmation := inliner.ask("t-open")
			before := inliner.resolvedThreadComments()
			command := testCase.start(inliner)
			inliner.replyElsewhere()
			read := messages(command) // read before the write, lands after it
			inliner.run(confirmation.OnYes)
			for _, message := range read {
				inliner.send(message)
			}
			if !inliner.threadState("t-open").IsResolved {
				test.Error("the resolution was lost")
			}
			if inliner.resolvedThreadComments() != before+1 {
				test.Errorf("the load's data was dropped: %d comments", inliner.resolvedThreadComments())
			}
			if inliner.view.threadPending {
				test.Error("threads still pending")
			}
		})
	}
}

// Of two overlapping thread loads only the later one counts, whatever the
// order they land in.
func TestOlderOverlappingThreadLoadIsDiscarded(test *testing.T) {
	inliner := newInliner(test)
	before := inliner.resolvedThreadComments()
	older := inliner.view.reloadThreads()()
	inliner.replyElsewhere()
	newer := inliner.view.reloadThreads()()
	inliner.send(newer)
	inliner.send(older)
	if inliner.resolvedThreadComments() != before+1 {
		test.Errorf("the older load overwrote the newer: %d comments", inliner.resolvedThreadComments())
	}
}
