package workspace

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

const (
	firstOwnIdentifier  = "c-first"
	secondOwnIdentifier = "c-second"
)

// seedTargets gives the viewer a thread of two published comments on new line
// 10, and a second thread on that line, besides the shared threads: t-open
// (two comments by others, line 14), t-resolved (folded, line 12), and
// nothing on line 11.
func (inliner *inliner) seedTargets() {
	inliner.test.Helper()
	thread := inliner.mine(true, true,
		github.ReviewComment{Identifier: firstOwnIdentifier, Body: "first mine"},
		github.ReviewComment{Identifier: secondOwnIdentifier, Body: "second mine"},
	)
	extra := github.ReviewThread{
		Identifier: "t-extra", Path: clientPath, Line: 10, DiffSide: github.SideRight,
		Comments: []github.ReviewComment{newComment("bob", "also here", 1)},
	}
	inliner.seed(thread, extra)
}

// targetPositions are the places the diff cursor can be, and how to get there.
var targetPositions = []struct {
	name   string
	moveTo func(inliner *inliner)
}{
	{"own comment", func(inliner *inliner) { inliner.toLineContaining("first mine") }},
	{"someone else's comment", func(inliner *inliner) { inliner.toComment(0) }},
	{"thread header", func(inliner *inliner) {
		inliner.toThread("t-open")
		inliner.diffView().CursorDown(1)
	}},
	{"separator row", func(inliner *inliner) { inliner.toLineContaining("") }},
	{"diff line with no thread", func(inliner *inliner) { inliner.moveTo(github.SideRight, 11) }},
	{"diff line with one thread", func(inliner *inliner) { inliner.toThread("t-open") }},
	{"diff line with two threads", func(inliner *inliner) { inliner.moveTo(github.SideRight, 10) }},
	{"folded thread", func(inliner *inliner) { inliner.toThread("t-resolved") }},
	{"files pane", func(inliner *inliner) {
		inliner.toComment(0)
		inliner.press("h")
	}},
}

// targetActions are the actions that work on a thread or a comment.
var targetActions = []struct {
	name string
	// run starts the action.
	run func(inliner *inliner)
	// notices are what it says by position, or nothing where it goes ahead.
	notices map[string]string
}{
	{
		"r", func(inliner *inliner) { inliner.press("r") },
		threadNotices("reply", ""),
	},
	{
		"x", func(inliner *inliner) { inliner.press("x") },
		threadNotices("resolve it", ""),
	},
	{
		":resolve", func(inliner *inliner) { inliner.command("resolve") },
		threadNotices("resolve it", "thread is already resolved"),
	},
	{
		":unresolve", func(inliner *inliner) { inliner.command("unresolve") },
		withNotice(threadNotices("resolve it", "thread is not resolved"), "folded thread", ""),
	},
	{
		":react", func(inliner *inliner) { inliner.react("heart") },
		commentNotices("react to it", "move onto a comment to react to it", false),
	},
	{
		"e", func(inliner *inliner) { inliner.press("e") },
		commentNotices("edit it", noticeEdit, true),
	},
	{
		"D", func(inliner *inliner) { inliner.press("D") },
		commentNotices("delete it", noticeDelete, true),
	},
	{
		":edit-comment", func(inliner *inliner) { inliner.command("edit-comment") },
		commentNotices("edit it", noticeEdit, true),
	},
	{
		":delete-comment", func(inliner *inliner) { inliner.command("delete-comment") },
		commentNotices("delete it", noticeDelete, true),
	},
}

// threadNotices is what a thread action says in each position. otherwise is
// what it says on every position that has a thread; "" goes ahead.
func threadNotices(words, folded string) map[string]string {
	notices := map[string]string{
		"diff line with no thread":   "no comment thread here",
		"diff line with two threads": "several threads on this line: move onto one to " + words,
		"files pane":                 "focus the diff to " + words,
	}
	if folded != "" {
		notices["folded thread"] = folded
	}
	// A thread that is not resolved is refused by :unresolve everywhere else.
	if folded == "thread is not resolved" {
		for _, name := range []string{"own comment", "someone else's comment", "thread header", "separator row", "diff line with one thread"} {
			notices[name] = folded
		}
		delete(notices, "folded thread")
	}
	return notices
}

func withNotice(notices map[string]string, position, notice string) map[string]string {
	if notice == "" {
		delete(notices, position)
	} else {
		notices[position] = notice
	}
	return notices
}

// commentNotices is what a comment action says in each position. Only the
// viewer's own comment goes ahead when own is true; any comment otherwise.
func commentNotices(words, notice string, own bool) map[string]string {
	notices := map[string]string{"files pane": "focus the diff to " + words}
	for _, position := range []string{"thread header", "separator row", "diff line with no thread", "diff line with one thread", "diff line with two threads", "folded thread"} {
		notices[position] = notice
	}
	if own {
		notices["someone else's comment"] = notice
	}
	return notices
}

// effect reports whether the action went ahead: it opened the editor, asked a
// question, or wrote a reaction.
func effect(inliner *inliner) bool {
	_, asked := inliner.lastConfirm()
	return len(inliner.openedOn) > 0 || asked || len(inliner.reactionCalls()) > 0
}

func TestEachActionResolvesItsTargetFromTheCursor(test *testing.T) {
	for _, action := range targetActions {
		for _, position := range targetPositions {
			test.Run(action.name+" on "+position.name, func(test *testing.T) {
				inliner := newInliner(test)
				inliner.seedTargets()
				position.moveTo(inliner)
				inliner.messages, inliner.openedOn, inliner.body = nil, nil, "text"
				action.run(inliner)
				want := action.notices[position.name]
				if want == "" {
					if !effect(inliner) {
						test.Errorf("did not go ahead; infos %v", inliner.infoMessages())
					}
					return
				}
				if diff := cmp.Diff([]string{want}, inliner.infoMessages()); diff != "" {
					test.Errorf("infos (-want +got):\n%s", diff)
				}
				if effect(inliner) {
					test.Error("went ahead although it was refused")
				}
			})
		}
	}
}

// e and D act on the comment under the cursor, not on the viewer's newest
// comment of the thread.
func TestEditAndDeleteActOnTheCommentUnderTheCursor(test *testing.T) {
	test.Run("edit the first of two", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seedTargets()
		inliner.toLineContaining("first mine")
		inliner.body = "edited"
		inliner.press("e")
		if diff := cmp.Diff([]string{"first mine"}, inliner.openedOn); diff != "" {
			test.Errorf("editor opened on (-want +got):\n%s", diff)
		}
	})
	test.Run("edit the second of two", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seedTargets()
		inliner.toLineContaining("second mine")
		inliner.body = "edited"
		inliner.press("e")
		if diff := cmp.Diff([]string{"second mine"}, inliner.openedOn); diff != "" {
			test.Errorf("editor opened on (-want +got):\n%s", diff)
		}
	})
	test.Run("delete the first of two", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seedTargets()
		inliner.toLineContaining("first mine")
		inliner.press("D")
		confirmation, ok := inliner.lastConfirm()
		if !ok {
			test.Fatal("nothing asked")
		}
		inliner.run(confirmation.OnYes)
		if diff := cmp.Diff([]string{firstOwnIdentifier}, inliner.client.DeletedComments()); diff != "" {
			test.Errorf("deleted (-want +got):\n%s", diff)
		}
	})
}

// The lines level needs no focus on the diff: c works from the Files pane.
func TestNewCommentWorksFromTheFilesPane(test *testing.T) {
	inliner := newInliner(test)
	inliner.moveTo(github.SideRight, 11)
	inliner.press("h")
	if inliner.view.focus != paneFiles {
		test.Fatalf("focus = %v, want the Files pane", inliner.view.focus)
	}
	inliner.body = "from the files pane"
	inliner.press("c")
	if diff := cmp.Diff([]string{""}, inliner.openedOn); diff != "" {
		test.Errorf("editor opened on (-want +got):\n%s", diff)
	}
}

// The Overview has no diff cursor, but the pane behind it remembers where it
// was: no action may target the thread or comment left there.
func TestOverviewNeverTargetsTheHiddenDiffCursor(test *testing.T) {
	tests := []struct {
		name string
		run  func(inliner *inliner)
		want string
	}{
		{"x", func(inliner *inliner) { inliner.press("x") }, "no comment thread here"},
		{"r", func(inliner *inliner) { inliner.press("r") }, "no comment thread here"},
		{"e", func(inliner *inliner) { inliner.press("e") }, noticeEdit},
		{"D", func(inliner *inliner) { inliner.press("D") }, noticeDelete},
		{":react", func(inliner *inliner) { inliner.react("heart") }, "move onto a comment to react to it"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.seedTargets()
			inliner.toLineContaining("first mine")
			inliner.press("h", "g g", "l") // Overview, right pane focused
			if inliner.view.selection() != 0 || inliner.view.focus != paneRight {
				test.Fatalf("selection %d, focus %v", inliner.view.selection(), inliner.view.focus)
			}
			inliner.messages, inliner.openedOn, inliner.body = nil, nil, "text"
			testCase.run(inliner)
			if diff := cmp.Diff([]string{testCase.want}, inliner.infoMessages()); diff != "" {
				test.Errorf("infos (-want +got):\n%s", diff)
			}
			if effect(inliner) {
				test.Error("went ahead on the hidden cursor")
			}
		})
	}
}
