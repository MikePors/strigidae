package diffview

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// contextView is the part of a Context the table compares: the thread
// identifiers rather than the threads.
type contextView struct {
	HasFile      bool
	InHunk       bool
	Hunk         int
	HasLines     bool
	Lines        Range
	Threads      []string
	OnComment    bool
	CommentIndex int
}

func viewOf(context Context) contextView {
	view := contextView{
		HasFile: context.File != nil, InHunk: context.InHunk, Hunk: context.Hunk,
		HasLines: context.HasLines, Lines: context.Lines,
		OnComment: context.OnComment, CommentIndex: context.CommentIndex,
	}
	for _, reviewThread := range context.Threads {
		view.Threads = append(view.Threads, reviewThread.Identifier)
	}
	return view
}

// separatorOffset is the offset below the header of the thread "open" of the
// blank row between its two comments.
func separatorOffset(test *testing.T, model *Model) int {
	test.Helper()
	for threadIndex, thread := range model.fileThreads {
		if thread.Identifier != "open" {
			continue
		}
		for offset, owner := range model.owners[threadIndex] {
			if offset > 0 && owner < 0 {
				return offset
			}
		}
	}
	test.Fatal("no separator row in the open thread")
	return 0
}

func TestContext(test *testing.T) {
	logRange := Range{Path: "server.go", Side: github.SideRight, Start: 5, End: 5}
	onLog := func(test *testing.T) *Model {
		model := threadModel(test, 100, 80)
		model.goToRow(test, `"log"`)
		return model
	}
	tests := []struct {
		name  string
		setup func(test *testing.T) *Model
		want  contextView
	}{
		{"no file", func(*testing.T) *Model { return New(theme.New(true)) }, contextView{}},
		{"diff line without threads", func(test *testing.T) *Model {
			model := threadModel(test, 100, 80)
			model.CursorDown(1)
			return model
		}, contextView{HasFile: true, InHunk: true, HasLines: true, Lines: Range{Path: "server.go", Side: github.SideRight, Start: 1, End: 1}}},
		{"hunk header", func(test *testing.T) *Model { return threadModel(test, 100, 80) }, contextView{HasFile: true, InHunk: true}},
		{"diff line with one thread", onLog, contextView{HasFile: true, InHunk: true, HasLines: true, Lines: logRange, Threads: []string{"open"}}},
		{"diff line with two threads", func(test *testing.T) *Model {
			model := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
			model.SetFile(fixture(test, "server.go"))
			second := sampleThreads()[0]
			second.Identifier = "second"
			model.SetThreads(append(sampleThreads(), second))
			model.SetSize(100, 80)
			model.goToRow(test, `"log"`)
			return model
		}, contextView{HasFile: true, InHunk: true, HasLines: true, Lines: logRange, Threads: []string{"open", "second"}}},
		{"thread header row", func(test *testing.T) *Model {
			model := onLog(test)
			model.CursorDown(1)
			return model
		}, contextView{HasFile: true, InHunk: true, Threads: []string{"open"}}},
		{"first comment row", func(test *testing.T) *Model {
			model := onLog(test)
			model.CursorDown(2)
			return model
		}, contextView{HasFile: true, InHunk: true, Threads: []string{"open"}, OnComment: true}},
		{"second comment row", func(test *testing.T) *Model {
			model := onLog(test)
			model.CursorDown(1 + separatorOffset(test, model) + 1)
			return model
		}, contextView{HasFile: true, InHunk: true, Threads: []string{"open"}, OnComment: true, CommentIndex: 1}},
		{"separator row between comments", func(test *testing.T) *Model {
			model := onLog(test)
			model.CursorDown(1 + separatorOffset(test, model))
			return model
		}, contextView{HasFile: true, InHunk: true, Threads: []string{"open"}}},
		{"folded thread", func(test *testing.T) *Model {
			model := onLog(test)
			model.CursorDown(1)
			model.CloseThread()
			return model
		}, contextView{HasFile: true, InHunk: true, Threads: []string{"open"}}},
		{"hidden resolved thread", func(test *testing.T) *Model {
			model := threadModel(test, 100, 80)
			model.SetHideResolved(true)
			model.CursorDown(2)
			return model
		}, contextView{HasFile: true, InHunk: true, HasLines: true, Lines: Range{Path: "server.go", Side: github.SideRight, Start: 2, End: 2}}},
		{"visual range", func(test *testing.T) *Model {
			model := onLog(test)
			model.VisualStart()
			if err := model.VisualMove(1); err != nil {
				test.Fatal(err)
			}
			return model
		}, contextView{HasFile: true, InHunk: true, HasLines: true, Lines: Range{Path: "server.go", Side: github.SideRight, Start: 5, End: 6}}},
		{"thread on no line", func(test *testing.T) *Model {
			model := threadModel(test, 100, 80)
			model.GotoBottom()
			return model
		}, contextView{HasFile: true, Threads: []string{"gone"}, OnComment: true}},
		{"notice row", func(test *testing.T) *Model {
			model := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
			model.SetSize(60, 10)
			model.SetFile(fixture(test, "logo.png"))
			model.SetThreads([]github.ReviewThread{binaryThread()})
			return model
		}, contextView{HasFile: true}},
		{"thread after a notice", func(test *testing.T) *Model {
			model := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
			model.SetSize(60, 10)
			model.SetFile(fixture(test, "logo.png"))
			model.SetThreads([]github.ReviewThread{binaryThread()})
			model.GotoBottom()
			return model
		}, contextView{HasFile: true, Threads: []string{"bin"}, OnComment: true}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got := viewOf(testCase.setup(test).Context())
			if difference := cmp.Diff(testCase.want, got); difference != "" {
				test.Errorf("Context (-want +got):\n%s", difference)
			}
		})
	}
}
