package diffview

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
)

// Rows of server.go (see diffview_test.go): 14 is a context line (old 31,
// new 32), 15 and 16 are deleted lines (old 32, 33), 17 and 18 are added
// lines (new 33, 34), and row 11 is the last line of hunk 1.
func TestAnchorSingleLine(test *testing.T) {
	tests := []struct {
		name string
		row  int
		want Range
		ok   bool
	}{
		{"context line is on the new side", 14, Range{Path: "server.go", Side: github.SideRight, Start: 32, End: 32}, true},
		{"deleted line is on the old side", 15, Range{Path: "server.go", Side: github.SideLeft, Start: 32, End: 32}, true},
		{"added line is on the new side", 17, Range{Path: "server.go", Side: github.SideRight, Start: 33, End: 33}, true},
		{"hunk header has none", hunk2Row, Range{}, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := newModel(test, 80, 30)
			model.CursorDown(testCase.row)
			got, ok := model.Anchor()
			if ok != testCase.ok {
				test.Fatalf("ok = %v", ok)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("anchor (-want +got):\n%s", diff)
			}
		})
	}
}

func TestVisualRanges(test *testing.T) {
	tests := []struct {
		name    string
		start   int
		move    int
		want    Range
		refusal error
		endRow  int
	}{
		{"context to context stays on the new side", 13, 1, Range{Path: "server.go", Side: github.SideRight, Start: 31, End: 32}, nil, 14},
		{"deleted lines are on the old side", 15, 1, Range{Path: "server.go", Side: github.SideLeft, Start: 32, End: 33}, nil, 16},
		{"context with deleted is on the old side", 14, 1, Range{Path: "server.go", Side: github.SideLeft, Start: 31, End: 32}, nil, 15},
		{"added lines", 17, 1, Range{Path: "server.go", Side: github.SideRight, Start: 33, End: 34}, nil, 18},
		{"upwards", 16, -1, Range{Path: "server.go", Side: github.SideLeft, Start: 32, End: 33}, nil, 15},
		{"deleted to added is refused", 16, 1, Range{Path: "server.go", Side: github.SideLeft, Start: 33, End: 33}, ErrMixesSides, 16},
		{"into the next hunk is refused", 11, 1, Range{Path: "server.go", Side: github.SideRight, Start: 10, End: 10}, ErrCrossesHunk, 11},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := newModel(test, 80, 30)
			model.CursorDown(testCase.start)
			if !model.VisualStart() || !model.VisualActive() {
				test.Fatal("visual mode did not start")
			}
			if got := model.VisualMove(testCase.move); !errors.Is(got, testCase.refusal) {
				test.Errorf("refusal = %v, want %v", got, testCase.refusal)
			}
			if got := model.window.Cursor(); got != testCase.endRow {
				test.Errorf("cursor row = %d, want %d", got, testCase.endRow)
			}
			got, ok := model.Anchor()
			if !ok {
				test.Fatal("no anchor")
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("range (-want +got):\n%s", diff)
			}
		})
	}
}

func TestVisualStartNeedsALine(test *testing.T) {
	model := newModel(test, 80, 30)
	if model.VisualStart() { // row 0 is a hunk header
		test.Error("visual mode started on a hunk header")
	}
	model.CursorDown(14)
	model.VisualStart()
	model.VisualEnd()
	if model.VisualActive() {
		test.Error("VisualEnd left the mode active")
	}
	model.VisualStart()
	model.SetFile(fixture(test, "server.go")) // a re-parse rebuilds the rows
	if model.VisualActive() {
		test.Error("a new file kept the selection")
	}
}

func TestVisualRendering(test *testing.T) {
	model := newModel(test, 80, 30)
	model.CursorDown(15)
	model.VisualStart()
	_ = model.VisualMove(1) // the move is valid; the rendering is what is checked
	lines := strings.Split(ansi.Strip(model.Render(80, 30)), "\n")
	if !strings.HasPrefix(lines[15], markSelected) || !strings.HasPrefix(lines[16], ">") {
		test.Errorf("selected rows not marked: %q / %q", lines[15], lines[16])
	}
	if strings.HasPrefix(lines[14], markSelected) || strings.HasPrefix(lines[17], markSelected) {
		test.Error("rows outside the selection are marked")
	}
	// The selected rows get the cursor tint, built here from the theme's own
	// styles rather than the renderer's precomputed set.
	theme := model.theme
	tint := theme.DiffCursor.GetBackground()
	gutter := func(mark string, old int, selected bool) string {
		style := theme.DiffGutterDelete
		if selected {
			style = style.Background(tint)
		}
		return style.Render(mark + numberCell(old, model.numberWidth) + " " + numberCell(0, model.numberWidth) + " ")
	}
	raw := strings.Split(model.Render(80, 30), "\n")
	if !strings.Contains(raw[15], gutter(markSelected, 32, true)) {
		test.Errorf("selected row is not tinted: %q", raw[15])
	}
	if !strings.Contains(raw[16], gutter(">", 33, true)) {
		test.Errorf("cursor row is not tinted: %q", raw[16])
	}
	if strings.Contains(raw[15], gutter(markSelected, 32, false)) {
		test.Error("selected row is drawn without the tint")
	}
}

func TestRangeFor(test *testing.T) {
	makeLine := func(kind diff.LineKind, oldNumber, newNumber int) *diff.Line {
		return &diff.Line{Kind: kind, Old: oldNumber, New: newNumber}
	}
	ctx, add, deleted := diff.Context, diff.Add, diff.Delete
	tests := []struct {
		name  string
		lines []*diff.Line
		want  Range
		err   error
	}{
		{"one context line is on the new side", []*diff.Line{makeLine(ctx, 4, 5)}, Range{Path: "f.go", Side: github.SideRight, Start: 5, End: 5}, nil},
		{"one deleted line is on the old side", []*diff.Line{makeLine(deleted, 4, 0)}, Range{Path: "f.go", Side: github.SideLeft, Start: 4, End: 4}, nil},
		{"context and added lines use new numbers", []*diff.Line{makeLine(ctx, 4, 5), makeLine(add, 0, 6), makeLine(add, 0, 7)}, Range{Path: "f.go", Side: github.SideRight, Start: 5, End: 7}, nil},
		{"any deleted line makes it the old side", []*diff.Line{makeLine(ctx, 4, 5), makeLine(deleted, 5, 0)}, Range{Path: "f.go", Side: github.SideLeft, Start: 4, End: 5}, nil},
		{"added with deleted is refused", []*diff.Line{makeLine(deleted, 5, 0), makeLine(add, 0, 6)}, Range{}, ErrMixesSides},
		{"no lines", nil, Range{}, errNoLines},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got, err := rangeFor("f.go", testCase.lines)
			if !errors.Is(err, testCase.err) {
				test.Fatalf("err = %v, want %v", err, testCase.err)
			}
			if difference := cmp.Diff(testCase.want, got); difference != "" {
				test.Errorf("range (-want +got):\n%s", difference)
			}
		})
	}
}

// furthest on server.go: rows 11 (last of hunk 1), 13..14 context, 15..16
// deleted, 17..18 added (see diffview_test.go).
func TestFurthest(test *testing.T) {
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
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := newModel(test, 80, 30)
			model.CursorDown(testCase.anchor)
			if !model.VisualStart() {
				test.Fatal("no visual start")
			}
			row, err := model.furthest(testCase.anchor, testCase.delta)
			if row != testCase.wantRow || !errors.Is(err, testCase.wantErr) {
				test.Errorf("furthest = %d, %v; want %d, %v", row, err, testCase.wantRow, testCase.wantErr)
			}
		})
	}
}
