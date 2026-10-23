package diffview

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

var threadNow = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func comment(author, body string, ago time.Duration) github.ReviewComment {
	return github.ReviewComment{
		Identifier: author + body, Author: author, Body: body, CreatedAt: threadNow.Add(-ago),
		Reactions: []github.ReactionSummary{{Content: github.ReactionThumbsUp, Count: 2}, {Content: github.ReactionRocket}},
	}
}

// sampleThreads are on server.go (see testdata/sample.diff): an open one on
// the added `log` import (new line 5), a resolved one on new line 2, one on
// the deleted line (old line 32), an outdated one with no current line, and
// one on a line that is not in the diff.
func sampleThreads() []github.ReviewThread {
	return []github.ReviewThread{
		{
			Identifier: "open", Path: "server.go", Line: 5, DiffSide: github.SideRight, SubjectType: github.SubjectLine,
			Comments: []github.ReviewComment{comment("alice", "Do we need **log** here?", 3*time.Hour), comment("bob", "Yes, for `Serve`.", time.Hour)},
		},
		{
			Identifier: "resolved", Path: "server.go", Line: 2, DiffSide: github.SideRight, IsResolved: true, ResolvedBy: "alice",
			Comments: []github.ReviewComment{comment("carol", "Blank line please.", 50*time.Hour)},
		},
		{
			Identifier: "left", Path: "server.go", Line: 32, DiffSide: github.SideLeft,
			Comments: []github.ReviewComment{comment("dave", "Why drop the return?", 2*time.Hour)},
		},
		{
			Identifier: "outdated", Path: "server.go", OriginalLine: 4, DiffSide: github.SideRight, IsOutdated: true,
			Comments: []github.ReviewComment{comment("erin", "Old thought.", 80*time.Hour)},
		},
		{
			Identifier: "gone", Path: "server.go", Line: 999, DiffSide: github.SideRight,
			Comments: []github.ReviewComment{comment("frank", "Where did this go?", time.Hour)},
		},
		{
			Identifier: "other-file", Path: "other.go", Line: 1, DiffSide: github.SideRight,
			Comments: []github.ReviewComment{comment("gina", "Not on this file.", time.Hour)},
		},
	}
}

func threadModel(test testing.TB, width, height int) *Model {
	test.Helper()
	model := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
	model.SetFile(fixture(test, "server.go"))
	model.SetThreads(sampleThreads())
	model.SetSize(width, height)
	return model
}

func screen(model *Model) string { return ansi.Strip(model.Render(model.width, model.height)) }

func cursorRowText(model *Model) string {
	lines := strings.Split(screen(model), "\n")
	return lines[model.window.Cursor()-model.window.Top()]
}

func TestThreadsPlacedByLineAndSide(test *testing.T) {
	model := threadModel(test, 100, 80)
	if model.ThreadCount() != 5 {
		test.Fatalf("threads on the file = %d, want 5 (other files' are ignored)", model.ThreadCount())
	}
	out := screen(model)
	lines := strings.Split(out, "\n")
	find := func(text string) int {
		for i, line := range lines {
			if strings.Contains(line, text) {
				return i
			}
		}
		test.Fatalf("%q not on screen:\n%s", text, out)
		return -1
	}
	// Each thread header sits right after its code line.
	if i := find("alice"); !strings.Contains(lines[i-1], `"log"`) {
		test.Errorf("open thread should follow the log line, follows %q", lines[i-1])
	}
	if i := find("dave"); !strings.Contains(lines[i-1], "fmt.Fprintln(w, \"hello\")") {
		test.Errorf("left-side thread should follow the deleted line, follows %q", lines[i-1])
	}
	// The outdated thread and the one on a missing line are listed after the last row.
	last := len(lines) - 1
	for _, who := range []string{"erin", "frank"} {
		if find(who) < find("// done") {
			test.Errorf("%s should come after the diff", who)
		}
	}
	_ = last
	if strings.Contains(out, "gina") {
		test.Error("a thread on another file was shown")
	}
}

func TestResolvedStartsCollapsedOpenStartsExpanded(test *testing.T) {
	out := screen(threadModel(test, 100, 80))
	if !strings.Contains(out, "▾ alice") || !strings.Contains(out, "Do we need log here?") {
		test.Errorf("open thread should be expanded with its body rendered:\n%s", out)
	}
	if !strings.Contains(out, "▸ carol 1 comment · 2d ago [resolved] — Blank line please.") {
		test.Errorf("resolved thread should be collapsed with a label and preview:\n%s", out)
	}
	if strings.Contains(out, "│ carol") {
		test.Error("resolved thread body shown while collapsed")
	}
	if !strings.Contains(out, "[outdated]") || !strings.Contains(out, "server.go:4") {
		test.Errorf("outdated thread should be labelled and located:\n%s", out)
	}
	if !strings.Contains(out, "👍 2") || strings.Contains(out, "🚀") {
		test.Errorf("reactions should show non-zero counts only:\n%s", out)
	}
}

func (model *Model) goToRow(test testing.TB, contains string) {
	test.Helper()
	for i, row := range model.rows {
		if row.threadRow == nil && row.line != nil && strings.Contains(row.text, contains) {
			model.window.JumpTo(i)
			return
		}
	}
	test.Fatalf("no diff row containing %q", contains)
}

func TestToggleExpandsAndCollapsesInPlace(test *testing.T) {
	model := threadModel(test, 100, 80)
	rowsOpen := len(model.rows)
	model.goToRow(test, `"log"`)
	cursorLine, _ := model.Cursor()
	model.ToggleThread()
	if len(model.rows) >= rowsOpen {
		test.Fatalf("collapsing should remove rows: %d -> %d", rowsOpen, len(model.rows))
	}
	if got, _ := model.Cursor(); got != cursorLine {
		test.Errorf("cursor moved off its diff line: %+v -> %+v", cursorLine, got)
	}
	if out := screen(model); strings.Contains(out, "│ alice") || !strings.Contains(out, "▸ alice") {
		test.Errorf("thread should be collapsed:\n%s", out)
	}
	model.ToggleThread()
	if len(model.rows) != rowsOpen {
		test.Errorf("rows after reopening = %d, want %d", len(model.rows), rowsOpen)
	}
	model.CloseThread()
	model.CloseThread() // idempotent
	model.OpenThread()
	model.OpenThread()
	if len(model.rows) != rowsOpen {
		test.Errorf("rows after close/open = %d, want %d", len(model.rows), rowsOpen)
	}
}

func TestToggleFromInsideTheThreadLandsOnHeader(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	model.CursorDown(3) // inside the expanded thread (header + comments)
	if model.rows[model.window.Cursor()].threadRow == nil {
		test.Fatal("setup: cursor should be on a thread row")
	}
	model.ToggleThread()
	if row := model.rows[model.window.Cursor()]; row.threadRow == nil || row.threadRow.rowInBlock != 0 || !strings.Contains(cursorRowText(model), "▸ alice") {
		test.Errorf("cursor should sit on the collapsed header, got %q", cursorRowText(model))
	}
	model.ToggleThread()
	if !strings.Contains(screen(model), "│ alice") {
		test.Error("toggling on the header should reopen the thread")
	}
}

func TestToggleWithoutThreadIsNoop(test *testing.T) {
	model := threadModel(test, 100, 80)
	count := len(model.rows)
	model.GotoTop() // hunk header
	model.ToggleThread()
	model.OpenThread()
	model.CloseThread()
	if len(model.rows) != count {
		test.Errorf("rows changed with no thread under the cursor: %d -> %d", count, len(model.rows))
	}
}

func TestThreadNavigation(test *testing.T) {
	model := threadModel(test, 100, 10)
	var seen []string
	for range 6 {
		model.NextThread()
		seen = append(seen, strings.TrimSpace(cursorRowText(model)))
	}
	joined := strings.Join(seen, "\n")
	for _, who := range []string{"carol", "alice", "dave", "erin", "frank"} {
		if !strings.Contains(joined, who) {
			test.Errorf("] c never reached %s:\n%s", who, joined)
		}
	}
	// The sixth press at the last thread stays on it.
	if !strings.Contains(seen[5], "frank") {
		test.Errorf("] c past the last thread should stay, at %q", seen[5])
	}
	for range 10 {
		model.PreviousThread()
	}
	if !strings.Contains(cursorRowText(model), "carol") {
		test.Errorf("[ c should stop at the first thread, at %q", cursorRowText(model))
	}
	if model.window.Cursor() < model.window.Top() || model.window.Cursor() >= model.window.Top()+model.height {
		test.Error("cursor left the window")
	}
}

func TestScrollingAccountsForThreadRows(test *testing.T) {
	model := threadModel(test, 100, 8)
	model.GotoBottom()
	if model.window.Cursor() != len(model.rows)-1 {
		test.Fatalf("cursor %d, rows %d", model.window.Cursor(), len(model.rows))
	}
	if !strings.Contains(screen(model), "Where did this go?") && !strings.Contains(screen(model), "frank") {
		test.Errorf("the last row should be visible:\n%s", screen(model))
	}
	if count := len(strings.Split(model.Render(100, 8), "\n")); count != 8 {
		test.Errorf("render height = %d", count)
	}
	model.GotoTop()
	for range len(model.rows) {
		model.CursorDown(1)
		if model.window.Cursor() < model.window.Top() || model.window.Cursor() >= model.window.Top()+8 {
			test.Fatalf("cursor %d outside window at top %d", model.window.Cursor(), model.window.Top())
		}
	}
}

func TestThreadRowsAreNotDiffLines(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	model.CursorDown(1)
	if _, ok := model.Cursor(); ok {
		test.Error("Cursor() should report no diff line on a thread row")
	}
}

func TestRefreshKeepsPlaceAndExpansion(test *testing.T) {
	model := threadModel(test, 100, 12)
	model.goToRow(test, "fmt.Fprintln(w, \"こんにちは")
	want, _ := model.Cursor()
	model.CloseThread() // not on a thread line: no-op; collapse the open one explicitly below
	model.goToRow(test, `"log"`)
	model.CloseThread()
	model.goToRow(test, "fmt.Fprintln(w, \"こんにちは")
	model.SetFile(fixture(test, "server.go")) // a re-parsed copy
	got, ok := model.Cursor()
	if !ok || got.Text != want.Text {
		test.Errorf("cursor after refresh = %+v, want %+v", got, want)
	}
	if strings.Contains(screen(model), "│ alice") {
		test.Error("collapsed state should survive a refresh")
	}
}

func TestMarkerGlyphs(test *testing.T) {
	model := threadModel(test, 100, 80)
	lines := strings.Split(screen(model), "\n")
	var marked []string
	for _, line := range lines {
		if strings.Contains(line, markerOpen) || strings.Contains(line, markerQuiet) {
			marked = append(marked, line)
		}
	}
	if len(marked) != 3 {
		test.Fatalf("marked lines = %d, want 3:\n%s", len(marked), strings.Join(marked, "\n"))
	}
	if !strings.Contains(marked[0], markerQuiet) || !strings.Contains(marked[1], markerOpen) || !strings.Contains(marked[2], markerOpen) {
		test.Errorf("resolved should be hollow, open filled:\n%s", strings.Join(marked, "\n"))
	}
}

func TestMarkerStyles(test *testing.T) {
	activeTheme := theme.New(true)
	model := threadModel(test, 100, 80)
	raw := model.Render(100, 80)
	// The open marker is drawn with the theme's ThreadMarker colour, the
	// resolved one with Muted, on top of whatever the row background is.
	open := activeTheme.ThreadMarker.Background(model.styles.variants[0][kindAdd].gutter.GetBackground()).Render(markerOpen)
	quiet := activeTheme.Muted.Background(model.styles.variants[0][kindContext].gutter.GetBackground()).Render(markerQuiet)
	if !strings.Contains(raw, open) {
		test.Error("open thread marker not drawn with the ThreadMarker style")
	}
	if !strings.Contains(raw, quiet) {
		test.Error("resolved thread marker not drawn with the Muted style")
	}
}

func TestOutdatedThreadIsMuted(test *testing.T) {
	activeTheme := theme.New(true)
	model := threadModel(test, 100, 80)
	want := activeTheme.Muted.Render("  Old thought.")
	if !strings.Contains(model.Render(100, 80), want) {
		test.Error("outdated body should be drawn in the Muted style")
	}
}

func TestGoldenThreads(test *testing.T) {
	model := threadModel(test, 70, 40)
	golden.Assert(test, "threads", screen(model), false)
	model.goToRow(test, `"log"`)
	model.ToggleThread()
	golden.Assert(test, "threads_collapsed", screen(model), false)
	narrowModel := threadModel(test, 40, 24)
	golden.Assert(test, "threads_narrow", screen(narrowModel), false)
}

func binaryThread() github.ReviewThread {
	return github.ReviewThread{
		Identifier: "bin", Path: "logo.png", SubjectType: github.SubjectFile,
		Comments: []github.ReviewComment{comment("hank", "Please compress.", time.Hour)},
	}
}

func TestThreadsOnBinaryFileInEitherOrder(test *testing.T) {
	for _, threadsFirst := range []bool{false, true} {
		model := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
		model.SetSize(60, 10)
		if threadsFirst {
			model.SetThreads([]github.ReviewThread{binaryThread()})
		}
		model.SetFile(fixture(test, "logo.png"))
		if !threadsFirst {
			model.SetThreads([]github.ReviewThread{binaryThread()})
		}
		out := screen(model)
		baseIndex, threadIndex := strings.Index(out, "Binary file"), strings.Index(out, "hank")
		if baseIndex < 0 || threadIndex < baseIndex {
			test.Errorf("threadsFirst=%v: notice should come first, then the thread:\n%s", threadsFirst, out)
		}
	}
}

func TestRefreshOfBinaryFileWithCursorOnThread(test *testing.T) {
	model := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
	model.SetSize(60, 10)
	model.SetFile(fixture(test, "logo.png"))
	model.SetThreads([]github.ReviewThread{binaryThread()})
	model.NextThread()
	model.SetMessage("Loading diff…") // must not panic
	model.SetFile(fixture(test, "logo.png"))
	if !strings.Contains(cursorRowText(model), "hank") {
		test.Errorf("cursor should be back on the thread, at %q", cursorRowText(model))
	}
	model.SetFile(nil)
	model.SetThreads(nil)
}

func TestCursorStaysOnItsThreadWhenThreadsChange(test *testing.T) {
	model := threadModel(test, 100, 12)
	model.NextThread()
	model.NextThread()
	model.NextThread() // dave, the thread on the deleted line
	if !strings.Contains(cursorRowText(model), "dave") {
		test.Fatalf("setup: at %q", cursorRowText(model))
	}
	model.CursorDown(1)
	added := github.ReviewThread{
		Identifier: "new", Path: "server.go", Line: 3, DiffSide: github.SideRight,
		Comments: []github.ReviewComment{comment("zed", "new above", time.Hour)},
	}
	model.SetThreads(append([]github.ReviewThread{added}, sampleThreads()...))
	if !strings.Contains(cursorRowText(model), "dave · 2h ago") {
		test.Errorf("cursor moved off its thread: %q", cursorRowText(model))
	}
	// And a refresh where the diff reloads first.
	model.SetFile(fixture(test, "server.go"))
	if !strings.Contains(cursorRowText(model), "dave · 2h ago") {
		test.Errorf("cursor left its thread on a diff refresh: %q", cursorRowText(model))
	}
}

func TestBlocksAreCachedPerThread(test *testing.T) {
	model := threadModel(test, 100, 40)
	base := model.renders
	model.goToRow(test, `"log"`)
	model.ToggleThread()
	if got := model.renders - base; got != 1 {
		test.Errorf("a toggle rendered %d blocks, want 1", got)
	}
	model.ToggleThread()
	if got := model.renders - base; got != 1 {
		test.Errorf("toggling back should hit the cache, rendered %d", got)
	}
	model.SetSize(90, 40)
	if model.renders-base < 1+model.ThreadCount() {
		test.Error("a width change should re-render the blocks")
	}
	before := model.renders
	model.SetThreads(sampleThreads())
	if model.renders == before {
		test.Error("SetThreads should drop the cache")
	}
}
