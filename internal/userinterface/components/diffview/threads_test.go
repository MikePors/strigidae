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

func threadModel(t testing.TB, w, h int) *Model {
	t.Helper()
	m := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
	m.SetFile(fixture(t, "server.go"))
	m.SetThreads(sampleThreads())
	m.SetSize(w, h)
	return m
}

func screen(m *Model) string { return ansi.Strip(m.Render(m.w, m.h)) }

func cursorRowText(m *Model) string {
	lines := strings.Split(screen(m), "\n")
	return lines[m.win.Cursor()-m.win.Top()]
}

func TestThreadsPlacedByLineAndSide(t *testing.T) {
	m := threadModel(t, 100, 80)
	if m.ThreadCount() != 5 {
		t.Fatalf("threads on the file = %d, want 5 (other files' are ignored)", m.ThreadCount())
	}
	out := screen(m)
	lines := strings.Split(out, "\n")
	find := func(s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		t.Fatalf("%q not on screen:\n%s", s, out)
		return -1
	}
	// Each thread header sits right after its code line.
	if i := find("alice"); !strings.Contains(lines[i-1], `"log"`) {
		t.Errorf("open thread should follow the log line, follows %q", lines[i-1])
	}
	if i := find("dave"); !strings.Contains(lines[i-1], "fmt.Fprintln(w, \"hello\")") {
		t.Errorf("left-side thread should follow the deleted line, follows %q", lines[i-1])
	}
	// The outdated thread and the one on a missing line are listed after the last row.
	last := len(lines) - 1
	for _, who := range []string{"erin", "frank"} {
		if find(who) < find("// done") {
			t.Errorf("%s should come after the diff", who)
		}
	}
	_ = last
	if strings.Contains(out, "gina") {
		t.Error("a thread on another file was shown")
	}
}

func TestResolvedStartsCollapsedOpenStartsExpanded(t *testing.T) {
	out := screen(threadModel(t, 100, 80))
	if !strings.Contains(out, "▾ alice") || !strings.Contains(out, "Do we need log here?") {
		t.Errorf("open thread should be expanded with its body rendered:\n%s", out)
	}
	if !strings.Contains(out, "▸ carol 1 comment · 2d ago [resolved] — Blank line please.") {
		t.Errorf("resolved thread should be collapsed with a label and preview:\n%s", out)
	}
	if strings.Contains(out, "│ carol") {
		t.Error("resolved thread body shown while collapsed")
	}
	if !strings.Contains(out, "[outdated]") || !strings.Contains(out, "server.go:4") {
		t.Errorf("outdated thread should be labelled and located:\n%s", out)
	}
	if !strings.Contains(out, "👍 2") || strings.Contains(out, "🚀") {
		t.Errorf("reactions should show non-zero counts only:\n%s", out)
	}
}

func (m *Model) goToRow(t testing.TB, contains string) {
	t.Helper()
	for i, r := range m.rows {
		if r.tr == nil && r.line != nil && strings.Contains(r.text, contains) {
			m.win.JumpTo(i)
			return
		}
	}
	t.Fatalf("no diff row containing %q", contains)
}

func TestToggleExpandsAndCollapsesInPlace(t *testing.T) {
	m := threadModel(t, 100, 80)
	rowsOpen := len(m.rows)
	m.goToRow(t, `"log"`)
	c, _ := m.Cursor()
	m.ToggleThread()
	if len(m.rows) >= rowsOpen {
		t.Fatalf("collapsing should remove rows: %d -> %d", rowsOpen, len(m.rows))
	}
	if got, _ := m.Cursor(); got != c {
		t.Errorf("cursor moved off its diff line: %+v -> %+v", c, got)
	}
	if out := screen(m); strings.Contains(out, "│ alice") || !strings.Contains(out, "▸ alice") {
		t.Errorf("thread should be collapsed:\n%s", out)
	}
	m.ToggleThread()
	if len(m.rows) != rowsOpen {
		t.Errorf("rows after reopening = %d, want %d", len(m.rows), rowsOpen)
	}
	m.CloseThread()
	m.CloseThread() // idempotent
	m.OpenThread()
	m.OpenThread()
	if len(m.rows) != rowsOpen {
		t.Errorf("rows after close/open = %d, want %d", len(m.rows), rowsOpen)
	}
}

func TestToggleFromInsideTheThreadLandsOnHeader(t *testing.T) {
	m := threadModel(t, 100, 80)
	m.goToRow(t, `"log"`)
	m.CursorDown(3) // inside the expanded thread (header + comments)
	if m.rows[m.win.Cursor()].tr == nil {
		t.Fatal("setup: cursor should be on a thread row")
	}
	m.ToggleThread()
	if r := m.rows[m.win.Cursor()]; r.tr == nil || r.tr.i != 0 || !strings.Contains(cursorRowText(m), "▸ alice") {
		t.Errorf("cursor should sit on the collapsed header, got %q", cursorRowText(m))
	}
	m.ToggleThread()
	if !strings.Contains(screen(m), "│ alice") {
		t.Error("toggling on the header should reopen the thread")
	}
}

func TestToggleWithoutThreadIsNoop(t *testing.T) {
	m := threadModel(t, 100, 80)
	n := len(m.rows)
	m.GotoTop() // hunk header
	m.ToggleThread()
	m.OpenThread()
	m.CloseThread()
	if len(m.rows) != n {
		t.Errorf("rows changed with no thread under the cursor: %d -> %d", n, len(m.rows))
	}
}

func TestThreadNavigation(t *testing.T) {
	m := threadModel(t, 100, 10)
	var seen []string
	for range 6 {
		m.NextThread()
		seen = append(seen, strings.TrimSpace(cursorRowText(m)))
	}
	joined := strings.Join(seen, "\n")
	for _, who := range []string{"carol", "alice", "dave", "erin", "frank"} {
		if !strings.Contains(joined, who) {
			t.Errorf("] c never reached %s:\n%s", who, joined)
		}
	}
	// The sixth press at the last thread stays on it.
	if !strings.Contains(seen[5], "frank") {
		t.Errorf("] c past the last thread should stay, at %q", seen[5])
	}
	for range 10 {
		m.PrevThread()
	}
	if !strings.Contains(cursorRowText(m), "carol") {
		t.Errorf("[ c should stop at the first thread, at %q", cursorRowText(m))
	}
	if m.win.Cursor() < m.win.Top() || m.win.Cursor() >= m.win.Top()+m.h {
		t.Error("cursor left the window")
	}
}

func TestScrollingAccountsForThreadRows(t *testing.T) {
	m := threadModel(t, 100, 8)
	m.GotoBottom()
	if m.win.Cursor() != len(m.rows)-1 {
		t.Fatalf("cursor %d, rows %d", m.win.Cursor(), len(m.rows))
	}
	if !strings.Contains(screen(m), "Where did this go?") && !strings.Contains(screen(m), "frank") {
		t.Errorf("the last row should be visible:\n%s", screen(m))
	}
	if n := len(strings.Split(m.Render(100, 8), "\n")); n != 8 {
		t.Errorf("render height = %d", n)
	}
	m.GotoTop()
	for range len(m.rows) {
		m.CursorDown(1)
		if m.win.Cursor() < m.win.Top() || m.win.Cursor() >= m.win.Top()+8 {
			t.Fatalf("cursor %d outside window at top %d", m.win.Cursor(), m.win.Top())
		}
	}
}

func TestThreadRowsAreNotDiffLines(t *testing.T) {
	m := threadModel(t, 100, 80)
	m.goToRow(t, `"log"`)
	m.CursorDown(1)
	if _, ok := m.Cursor(); ok {
		t.Error("Cursor() should report no diff line on a thread row")
	}
}

func TestRefreshKeepsPlaceAndExpansion(t *testing.T) {
	m := threadModel(t, 100, 12)
	m.goToRow(t, "fmt.Fprintln(w, \"こんにちは")
	want, _ := m.Cursor()
	m.CloseThread() // not on a thread line: no-op; collapse the open one explicitly below
	m.goToRow(t, `"log"`)
	m.CloseThread()
	m.goToRow(t, "fmt.Fprintln(w, \"こんにちは")
	m.SetFile(fixture(t, "server.go")) // a re-parsed copy
	got, ok := m.Cursor()
	if !ok || got.Text != want.Text {
		t.Errorf("cursor after refresh = %+v, want %+v", got, want)
	}
	if strings.Contains(screen(m), "│ alice") {
		t.Error("collapsed state should survive a refresh")
	}
}

func TestMarkerGlyphs(t *testing.T) {
	m := threadModel(t, 100, 80)
	lines := strings.Split(screen(m), "\n")
	var marked []string
	for _, l := range lines {
		if strings.Contains(l, markerOpen) || strings.Contains(l, markerQuiet) {
			marked = append(marked, l)
		}
	}
	if len(marked) != 3 {
		t.Fatalf("marked lines = %d, want 3:\n%s", len(marked), strings.Join(marked, "\n"))
	}
	if !strings.Contains(marked[0], markerQuiet) || !strings.Contains(marked[1], markerOpen) || !strings.Contains(marked[2], markerOpen) {
		t.Errorf("resolved should be hollow, open filled:\n%s", strings.Join(marked, "\n"))
	}
}

func TestMarkerStyles(t *testing.T) {
	th := theme.New(true)
	m := threadModel(t, 100, 80)
	raw := m.Render(100, 80)
	// The open marker is drawn with the theme's ThreadMarker colour, the
	// resolved one with Muted, on top of whatever the row background is.
	open := th.ThreadMarker.Background(m.styles.v[0][kindAdd].gutter.GetBackground()).Render(markerOpen)
	quiet := th.Muted.Background(m.styles.v[0][kindContext].gutter.GetBackground()).Render(markerQuiet)
	if !strings.Contains(raw, open) {
		t.Error("open thread marker not drawn with the ThreadMarker style")
	}
	if !strings.Contains(raw, quiet) {
		t.Error("resolved thread marker not drawn with the Muted style")
	}
}

func TestOutdatedThreadIsMuted(t *testing.T) {
	th := theme.New(true)
	m := threadModel(t, 100, 80)
	want := th.Muted.Render("  Old thought.")
	if !strings.Contains(m.Render(100, 80), want) {
		t.Error("outdated body should be drawn in the Muted style")
	}
}

func TestGoldenThreads(t *testing.T) {
	m := threadModel(t, 70, 40)
	golden.Assert(t, "threads", screen(m), false)
	m.goToRow(t, `"log"`)
	m.ToggleThread()
	golden.Assert(t, "threads_collapsed", screen(m), false)
	n := threadModel(t, 40, 24)
	golden.Assert(t, "threads_narrow", screen(n), false)
}

func binaryThread() github.ReviewThread {
	return github.ReviewThread{
		Identifier: "bin", Path: "logo.png", SubjectType: github.SubjectFile,
		Comments: []github.ReviewComment{comment("hank", "Please compress.", time.Hour)},
	}
}

func TestThreadsOnBinaryFileInEitherOrder(t *testing.T) {
	for _, threadsFirst := range []bool{false, true} {
		m := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
		m.SetSize(60, 10)
		if threadsFirst {
			m.SetThreads([]github.ReviewThread{binaryThread()})
		}
		m.SetFile(fixture(t, "logo.png"))
		if !threadsFirst {
			m.SetThreads([]github.ReviewThread{binaryThread()})
		}
		out := screen(m)
		bi, ti := strings.Index(out, "Binary file"), strings.Index(out, "hank")
		if bi < 0 || ti < bi {
			t.Errorf("threadsFirst=%v: notice should come first, then the thread:\n%s", threadsFirst, out)
		}
	}
}

func TestRefreshOfBinaryFileWithCursorOnThread(t *testing.T) {
	m := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
	m.SetSize(60, 10)
	m.SetFile(fixture(t, "logo.png"))
	m.SetThreads([]github.ReviewThread{binaryThread()})
	m.NextThread()
	m.SetMessage("Loading diff…") // must not panic
	m.SetFile(fixture(t, "logo.png"))
	if !strings.Contains(cursorRowText(m), "hank") {
		t.Errorf("cursor should be back on the thread, at %q", cursorRowText(m))
	}
	m.SetFile(nil)
	m.SetThreads(nil)
}

func TestCursorStaysOnItsThreadWhenThreadsChange(t *testing.T) {
	m := threadModel(t, 100, 12)
	m.NextThread()
	m.NextThread()
	m.NextThread() // dave, the thread on the deleted line
	if !strings.Contains(cursorRowText(m), "dave") {
		t.Fatalf("setup: at %q", cursorRowText(m))
	}
	m.CursorDown(1)
	added := github.ReviewThread{
		Identifier: "new", Path: "server.go", Line: 3, DiffSide: github.SideRight,
		Comments: []github.ReviewComment{comment("zed", "new above", time.Hour)},
	}
	m.SetThreads(append([]github.ReviewThread{added}, sampleThreads()...))
	if !strings.Contains(cursorRowText(m), "dave · 2h ago") {
		t.Errorf("cursor moved off its thread: %q", cursorRowText(m))
	}
	// And a refresh where the diff reloads first.
	m.SetFile(fixture(t, "server.go"))
	if !strings.Contains(cursorRowText(m), "dave · 2h ago") {
		t.Errorf("cursor left its thread on a diff refresh: %q", cursorRowText(m))
	}
}

func TestBlocksAreCachedPerThread(t *testing.T) {
	m := threadModel(t, 100, 40)
	base := m.renders
	m.goToRow(t, `"log"`)
	m.ToggleThread()
	if got := m.renders - base; got != 1 {
		t.Errorf("a toggle rendered %d blocks, want 1", got)
	}
	m.ToggleThread()
	if got := m.renders - base; got != 1 {
		t.Errorf("toggling back should hit the cache, rendered %d", got)
	}
	m.SetSize(90, 40)
	if m.renders-base < 1+m.ThreadCount() {
		t.Error("a width change should re-render the blocks")
	}
	before := m.renders
	m.SetThreads(sampleThreads())
	if m.renders == before {
		t.Error("SetThreads should drop the cache")
	}
}
