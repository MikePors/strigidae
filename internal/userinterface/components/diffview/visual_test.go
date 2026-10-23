package diffview

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
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
		{"context line is on the new side", 14, Range{Path: "server.go", Side: SideRight, Start: 32, End: 32}, true},
		{"deleted line is on the old side", 15, Range{Path: "server.go", Side: SideLeft, Start: 32, End: 32}, true},
		{"added line is on the new side", 17, Range{Path: "server.go", Side: SideRight, Start: 33, End: 33}, true},
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
		refusal error
		endRow  int
	}{
		{"context to context stays on the new side", 13, 1, Range{Path: "server.go", Side: SideRight, Start: 31, End: 32}, nil, 14},
		{"deleted lines are on the old side", 15, 1, Range{Path: "server.go", Side: SideLeft, Start: 32, End: 33}, nil, 16},
		{"context with deleted is on the old side", 14, 1, Range{Path: "server.go", Side: SideLeft, Start: 31, End: 32}, nil, 15},
		{"added lines", 17, 1, Range{Path: "server.go", Side: SideRight, Start: 33, End: 34}, nil, 18},
		{"upwards", 16, -1, Range{Path: "server.go", Side: SideLeft, Start: 32, End: 33}, nil, 15},
		{"deleted to added is refused", 16, 1, Range{Path: "server.go", Side: SideLeft, Start: 33, End: 33}, ErrMixesSides, 16},
		{"into the next hunk is refused", 11, 1, Range{Path: "server.go", Side: SideRight, Start: 10, End: 10}, ErrCrossesHunk, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(t, 80, 30)
			m.CursorDown(tt.start)
			if !m.VisualStart() || !m.VisualActive() {
				t.Fatal("visual mode did not start")
			}
			if got := m.VisualMove(tt.move); !errors.Is(got, tt.refusal) {
				t.Errorf("refusal = %v, want %v", got, tt.refusal)
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
	_ = m.VisualMove(1) // the move is valid; the rendering is what is checked
	lines := strings.Split(ansi.Strip(m.Render(80, 30)), "\n")
	if !strings.HasPrefix(lines[15], markSelected) || !strings.HasPrefix(lines[16], ">") {
		t.Errorf("selected rows not marked: %q / %q", lines[15], lines[16])
	}
	if strings.HasPrefix(lines[14], markSelected) || strings.HasPrefix(lines[17], markSelected) {
		t.Error("rows outside the selection are marked")
	}
	// The selected rows get the cursor tint, built here from the theme's own
	// styles rather than the renderer's precomputed set.
	th := m.th
	tint := th.DiffCursor.GetBackground()
	gutter := func(mark string, old int, selected bool) string {
		st := th.DiffGutterDelete
		if selected {
			st = st.Background(tint)
		}
		return st.Render(mark + numberCell(old, m.numberWidth) + " " + numberCell(0, m.numberWidth) + " ")
	}
	raw := strings.Split(m.Render(80, 30), "\n")
	if !strings.Contains(raw[15], gutter(markSelected, 32, true)) {
		t.Errorf("selected row is not tinted: %q", raw[15])
	}
	if !strings.Contains(raw[16], gutter(">", 33, true)) {
		t.Errorf("cursor row is not tinted: %q", raw[16])
	}
	if strings.Contains(raw[15], gutter(markSelected, 32, false)) {
		t.Error("selected row is drawn without the tint")
	}
}

func TestRangeFor(t *testing.T) {
	ln := func(k diff.LineKind, oldN, newN int) *diff.Line { return &diff.Line{Kind: k, Old: oldN, New: newN} }
	ctx, add, del := diff.Context, diff.Add, diff.Delete
	tests := []struct {
		name  string
		lines []*diff.Line
		want  Range
		err   error
	}{
		{"one context line is on the new side", []*diff.Line{ln(ctx, 4, 5)}, Range{Path: "f.go", Side: SideRight, Start: 5, End: 5}, nil},
		{"one deleted line is on the old side", []*diff.Line{ln(del, 4, 0)}, Range{Path: "f.go", Side: SideLeft, Start: 4, End: 4}, nil},
		{"context and added lines use new numbers", []*diff.Line{ln(ctx, 4, 5), ln(add, 0, 6), ln(add, 0, 7)}, Range{Path: "f.go", Side: SideRight, Start: 5, End: 7}, nil},
		{"any deleted line makes it the old side", []*diff.Line{ln(ctx, 4, 5), ln(del, 5, 0)}, Range{Path: "f.go", Side: SideLeft, Start: 4, End: 5}, nil},
		{"added with deleted is refused", []*diff.Line{ln(del, 5, 0), ln(add, 0, 6)}, Range{}, ErrMixesSides},
		{"no lines", nil, Range{}, errNoLines},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rangeFor("f.go", tt.lines)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("range (-want +got):\n%s", diff)
			}
		})
	}
}

// furthest on server.go: rows 11 (last of hunk 1), 13..14 context, 15..16
// deleted, 17..18 added (see diffview_test.go).
func TestFurthest(t *testing.T) {
	tests := []struct {
		name    string
		anchor  int
		delta   int
		wantRow int
		wantErr error
	}{
		{"all the way", 13, 2, 15, nil},
		{"no move", 14, 0, 14, nil},
		{"stops before mixing sides, partway", 16, 2, 16, ErrMixesSides},
		{"stops before the added line, after one step", 14, 3, 16, ErrMixesSides},
		{"stops at the hunk edge", 11, 1, 11, ErrCrossesHunk},
		{"upwards", 16, -1, 15, nil},
		{"runs out of rows without an error", 18, 50, 23, nil},
		{"upwards into the previous hunk", 13, -1, 13, ErrCrossesHunk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(t, 80, 30)
			m.CursorDown(tt.anchor)
			if !m.VisualStart() {
				t.Fatal("no visual start")
			}
			row, err := m.furthest(tt.anchor, tt.delta)
			if row != tt.wantRow || !errors.Is(err, tt.wantErr) {
				t.Errorf("furthest = %d, %v; want %d, %v", row, err, tt.wantRow, tt.wantErr)
			}
		})
	}
}
