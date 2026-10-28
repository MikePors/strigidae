package diffview

import (
	"errors"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// TestContextHunk walks every row of server.go with its threads: a header, a
// line or a thread under a line is in the hunk of the last header above it;
// the threads on no line (outdated, gone), shown at the end, are in none.
func TestContextHunk(test *testing.T) {
	model := threadModel(test, 100, 80)
	headers, threadRows, orphanRows := -1, 0, 0
	for i := range model.rows {
		model.GotoTop()
		model.CursorDown(i)
		context := model.Context()
		file, hunk, ok := context.File, context.Hunk, context.InHunk
		row := model.rows[i]
		if row.line == nil && row.threadRow == nil && !row.message {
			headers++
		}
		if row.threadRow != nil {
			threadRows++
			if !ok {
				orphanRows++
				continue
			}
		}
		if !ok || file != model.file || hunk != headers {
			test.Errorf("row %d: Context = %v, %d, %v; want hunk %d", i, file != nil, hunk, ok, headers)
		}
	}
	if headers != len(model.file.Hunks)-1 || threadRows == 0 || orphanRows == 0 {
		test.Fatalf("setup: %d headers, %d thread rows, %d orphan rows", headers+1, threadRows, orphanRows)
	}
	if model.rows[len(model.rows)-1].threadRow == nil {
		test.Error("setup: the orphan threads are not at the end")
	}
}

func TestContextHunkWithoutHunks(test *testing.T) {
	model := New(theme.New(true))
	if model.Context().InHunk {
		test.Error("no file: Context is ok")
	}
	model.SetFile(fixture(test, "server.go"))
	model.SetSize(80, 10)
	model.SetDiffError(errors.New("boom"))
	if model.Context().InHunk {
		test.Error("an error message is shown: Context is ok")
	}
}

// TestContextHunkOnNotice uses a rename with no hunks and a thread: its
// notice rows and the thread are in no hunk.
func TestContextHunkOnNotice(test *testing.T) {
	model := New(theme.New(true))
	model.SetFile(fixture(test, "pkg/new.go"))
	model.SetThreads([]github.ReviewThread{{
		Identifier: "t", Path: "pkg/new.go", Line: 1, DiffSide: github.SideRight,
		Comments: []github.ReviewComment{{Author: "a", Body: "renamed?"}},
	}})
	model.SetSize(80, 20)
	if len(model.rows) < 2 || !model.rows[0].message {
		test.Fatalf("setup: rows %+v", model.rows)
	}
	for i := range model.rows {
		model.GotoTop()
		model.CursorDown(i)
		if model.Context().InHunk {
			test.Errorf("row %d (%+v): Context is ok", i, model.rows[i])
		}
	}
}

func TestReviewedHunkMarker(test *testing.T) {
	model := newModel(test, 100, 40)
	model.SetHunkReviewed(func(file *diff.File, hunk int) bool { return file == model.file && hunk == 1 })
	lines := strings.Split(screen(model), "\n")
	if strings.Contains(lines[0], "✓") {
		test.Errorf("first hunk is marked: %q", lines[0])
	}
	if got := lines[hunk2Row]; !strings.HasPrefix(got, " ✓ @@") {
		test.Errorf("second hunk header = %q, want the reviewed marker", got)
	}
	if strings.Count(strings.Join(lines, "\n"), "✓") != 1 {
		test.Error("the marker appears more than once")
	}
}
