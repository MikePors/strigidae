package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

// Rows of server.go (see diffview_test.go): 14 is a context line (old 31,
// new 32), 15 and 16 are deleted lines (old 32, 33), 17 and 18 are added
// lines (new 33, 34), and row 11 is the last line of hunk 1.
func TestAnchorSingleLine(t *testing.T) {
	tests := []struct {
		name string
		row  int
		want Range
		ok   bool
	}{
		{"context line is on the new side", 14, Range{Path: "server.go", Hunk: 1, Side: SideRight, Start: 32, End: 32}, true},
		{"deleted line is on the old side", 15, Range{Path: "server.go", Hunk: 1, Side: SideLeft, Start: 32, End: 32}, true},
		{"added line is on the new side", 17, Range{Path: "server.go", Hunk: 1, Side: SideRight, Start: 33, End: 33}, true},
		{"hunk header has none", hunk2Row, Range{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(t, 80, 30)
			m.CursorDown(tt.row)
			got, ok := m.Anchor()
			if ok != tt.ok {
				t.Fatalf("ok = %v", ok)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("anchor (-want +got):\n%s", diff)
			}
		})
	}
}

func TestVisualRanges(t *testing.T) {
	tests := []struct {
		name    string
		start   int
		move    int
		want    Range
		refusal string
		endRow  int
	}{
		{"context to context stays on the new side", 13, 1, Range{Path: "server.go", Hunk: 1, Side: SideRight, Start: 31, End: 32}, "", 14},
		{"deleted lines are on the old side", 15, 1, Range{Path: "server.go", Hunk: 1, Side: SideLeft, Start: 32, End: 33}, "", 16},
		{"context with deleted is on the old side", 14, 1, Range{Path: "server.go", Hunk: 1, Side: SideLeft, Start: 31, End: 32}, "", 15},
		{"added lines", 17, 1, Range{Path: "server.go", Hunk: 1, Side: SideRight, Start: 33, End: 34}, "", 18},
		{"upwards", 16, -1, Range{Path: "server.go", Hunk: 1, Side: SideLeft, Start: 32, End: 33}, "", 15},
		{"deleted to added is refused", 16, 1, Range{Path: "server.go", Hunk: 1, Side: SideLeft, Start: 33, End: 33}, RefuseSide, 16},
		{"into the next hunk is refused", 11, 1, Range{Path: "server.go", Hunk: 0, Side: SideRight, Start: 10, End: 10}, RefuseHunk, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(t, 80, 30)
			m.CursorDown(tt.start)
			if !m.VisualStart() || !m.VisualActive() {
				t.Fatal("visual mode did not start")
			}
			if got := m.VisualMove(tt.move); got != tt.refusal {
				t.Errorf("refusal = %q, want %q", got, tt.refusal)
			}
			if got := m.win.Cursor(); got != tt.endRow {
				t.Errorf("cursor row = %d, want %d", got, tt.endRow)
			}
			got, ok := m.Anchor()
			if !ok {
				t.Fatal("no anchor")
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("range (-want +got):\n%s", diff)
			}
		})
	}
}

func TestVisualStartNeedsALine(t *testing.T) {
	m := newModel(t, 80, 30)
	if m.VisualStart() { // row 0 is a hunk header
		t.Error("visual mode started on a hunk header")
	}
	m.CursorDown(14)
	m.VisualStart()
	m.VisualEnd()
	if m.VisualActive() {
		t.Error("VisualEnd left the mode active")
	}
	m.VisualStart()
	m.SetFile(fixture(t, "server.go")) // a re-parse rebuilds the rows
	if m.VisualActive() {
		t.Error("a new file kept the selection")
	}
}

func TestVisualRendering(t *testing.T) {
	m := newModel(t, 80, 30)
	m.CursorDown(15)
	m.VisualStart()
	m.VisualMove(1)
	lines := strings.Split(ansi.Strip(m.Render(80, 30)), "\n")
	if !strings.HasPrefix(lines[15], markSelected) || !strings.HasPrefix(lines[16], ">") {
		t.Errorf("selected rows not marked: %q / %q", lines[15], lines[16])
	}
	if strings.HasPrefix(lines[14], markSelected) || strings.HasPrefix(lines[17], markSelected) {
		t.Error("rows outside the selection are marked")
	}
	// The tint is the cursor row's own style.
	th := m.th
	want := th.DiffCursor.GetBackground()
	if m.styles.v[1][kindDelete].gutter.GetBackground() != want {
		t.Error("selected rows do not use the cursor tint")
	}
}
