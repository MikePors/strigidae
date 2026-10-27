package diffview

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// relativeColumn returns the relative-number column of a screen line: the
// cells after the cursor mark.
func relativeColumn(line string) string {
	return strings.TrimSpace(ansi.Cut(line, 1, 1+relativeDigits))
}

// wantDistances checks every screen line of an unwrapped model shows its
// row's distance from the cursor.
func wantDistances(test *testing.T, model *Model) {
	test.Helper()
	lines := strings.Split(screen(model), "\n")
	for k, line := range lines {
		row := model.window.Top() + k
		if row >= len(model.rows) {
			break
		}
		if got, want := relativeColumn(line), strconv.Itoa(absolute(row-model.window.Cursor())); got != want {
			test.Errorf("screen line %d (row %d) shows %q, want %q:\n%s", k, row, got, want, line)
		}
	}
}

// diffWithEveryRowKind is server.go with its threads and its second hunk
// hidden, so diff lines, hunk headers, a placeholder and thread rows show.
func diffWithEveryRowKind(test *testing.T) *Model {
	model := hidingModel(test, map[int]bool{1: true}, nil)
	model.SetRelativeNumber(true)
	model.CursorDown(6)
	kinds := map[string]bool{}
	for _, row := range model.rows {
		switch {
		case row.threadRow != nil:
			kinds["thread"] = true
		case row.placeholder:
			kinds["placeholder"] = true
		case row.line == nil:
			kinds["hunk header"] = true
		default:
			kinds["diff line"] = true
		}
	}
	if len(kinds) != 4 {
		test.Fatalf("setup: row kinds shown = %v, want all four", kinds)
	}
	return model
}

// noticeWithThread is a file with no diff rows, shown at width, with a
// file-level thread so its notice is drawn as rows.
func noticeWithThread(test *testing.T, path string, width int) *Model {
	model := New(theme.New(true), WithClock(func() time.Time { return threadNow }), WithRelativeNumber(true))
	model.SetSize(width, 10)
	model.SetFile(fixture(test, path))
	thread := binaryThread()
	thread.Path = path
	model.SetThreads([]github.ReviewThread{thread})
	model.CursorDown(1)
	return model
}

// wantNoticeInFull checks the notice rows on screen, after the cursor mark
// and the column, hold the whole notice: none of it is cut off.
func wantNoticeInFull(test *testing.T, model *Model) {
	test.Helper()
	var shown []string
	for k, line := range strings.Split(screen(model), "\n") {
		if row := model.window.Top() + k; row < len(model.rows) && model.rows[row].message {
			shown = append(shown, strings.TrimSpace(ansi.Cut(line, 1+model.relativeColumnWidth(), model.width)))
		}
	}
	if shown == nil {
		return
	}
	if got, want := strings.Join(shown, " "), strings.Join(model.messageLines(), " "); got != want {
		test.Errorf("notice shows %q, want %q", got, want)
	}
}

func TestRelativeNumberOnEveryRow(test *testing.T) {
	for _, testCase := range []struct {
		name  string
		setup func(test *testing.T) *Model
	}{
		{"every row kind", diffWithEveryRowKind},
		{"binary notice", func(test *testing.T) *Model { return noticeWithThread(test, "logo.png", 60) }},
		{"wrapped rename notice", func(test *testing.T) *Model { return noticeWithThread(test, "pkg/new.go", 30) }},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			model := testCase.setup(test)
			wantDistances(test, model)
			for k, line := range strings.Split(screen(model), "\n") {
				if width := ansi.StringWidth(line); width != model.width {
					test.Errorf("screen line %d is %d cells wide, want %d", k, width, model.width)
				}
			}
			wantNoticeInFull(test, model)
		})
	}
}

func TestRelativeNumberLeavesContinuationRowsBlank(test *testing.T) {
	model := New(theme.New(true), WithWrap(true), WithRelativeNumber(true))
	model.SetFile(longLineFile())
	model.SetSize(26, 10)
	// Rows: the hunk header (the cursor), "short", the long added line over
	// four screen rows, "gone".
	want := []string{"0", "1", "2", "", "", "", "3"}
	lines := strings.Split(screen(model), "\n")
	for k, wantNumber := range want {
		if got := relativeColumn(lines[k]); got != wantNumber {
			test.Errorf("screen line %d shows %q, want %q:\n%s", k, got, wantNumber, screen(model))
		}
		if width := ansi.StringWidth(lines[k]); width != model.width {
			test.Errorf("screen line %d is %d cells wide, want %d", k, width, model.width)
		}
	}
}

func TestRelativeDigitsFitTheLargestCount(test *testing.T) {
	if want := len(strconv.Itoa(keys.MaximumCount)); relativeDigits != want {
		test.Errorf("relativeDigits = %d, want %d, the digits of keys.MaximumCount", relativeDigits, want)
	}
}

func TestRelativeCellBeyondCountsIsBlank(test *testing.T) {
	model := New(theme.New(true), WithRelativeNumber(true))
	if got := strings.TrimSpace(model.relativeCell(keys.MaximumCount)); got != strconv.Itoa(keys.MaximumCount) {
		test.Errorf("a row a count can reach shows %q", got)
	}
	if got := model.relativeCell(keys.MaximumCount + 1); strings.TrimSpace(got) != "" || len(got) != relativeDigits+1 {
		test.Errorf("a row beyond any count shows %q, want %d blank cells", got, relativeDigits+1)
	}
}

func TestRelativeNumberToggle(test *testing.T) {
	model := threadModel(test, 70, 40)
	before := screen(model)
	model.SetRelativeNumber(true)
	if model.gutterWidth() != 2*model.numberWidth+5+relativeDigits+1 {
		test.Errorf("gutter width = %d with the column shown", model.gutterWidth())
	}
	wantDistances(test, model)
	model.SetRelativeNumber(false)
	if after := screen(model); after != before {
		test.Errorf("turning the column off should restore the screen:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestGoldenRelativeNumber(test *testing.T) {
	model := threadModel(test, 70, 40)
	model.SetRelativeNumber(true)
	model.CursorDown(4)
	golden.Assert(test, "relative_number", screen(model), false)
}
