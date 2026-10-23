package diffview

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// hostileDiff carries terminal escapes, C1 controls, BEL and bidi overrides in
// every field a diff author controls.
const hostileDiff = "diff --git \"a/x\\033[2J\" \"b/y\\033]0;t\\007\"\n" +
	"old mode 100644\x1b[31m\n" +
	"new mode \u009b2J100755\n" +
	"similarity index 90%\n" +
	"rename from \"x\\033[2J\"\n" +
	"rename to \"y\\033]0;t\\007\"\n" +
	"diff --git a/h.go b/h.go\n" +
	"--- a/h.go\n" +
	"+++ b/h.go\n" +
	"@@ -1,2 +1,2 @@ func f() { // \x1b]52;c;cHduZWQ=\x07\x1b[2J \u009b31m \u202e\ttab\n" +
	" ctx \x1b[2J\u009d\x07 caf\xe9 \x9b[2J\xc3\n" +
	"-old\x1b]0;x\x1b\\ \t del\n" +
	"+new\u202e\u0085 \x1bP$q\x1b\\ add\n" +
	"@@ -9 +9 @@ \x1b[31m sec\x9b\xff\n" +
	"-a\n" +
	"+b\n" +
	"\\ No newline \x1b[2J\n" +
	"diff --git \"a/bin\\033[2J\" \"b/bin\\033[2J\"\n" +
	"copy from \"bin\\033[2J\"\n" +
	"copy to \"bin2\\033[2J\"\n" +
	"Binary files a/bin and b/bin2 differ\n"

func assertSafe(test *testing.T, label, out string) {
	test.Helper()
	if !utf8.ValidString(out) {
		test.Errorf("%s: output is not valid UTF-8: %q", label, out)
	}
	for _, character := range out {
		if character == '\n' {
			continue
		}
		if unicode.IsControl(character) || unicode.Is(unicode.Bidi_Control, character) {
			test.Errorf("%s: unsafe rune %U in %q", label, character, out)
			return
		}
	}
}

func TestHostileContentIsSanitised(test *testing.T) {
	parsedDiff, _ := diff.Parse(hostileDiff)
	if len(parsedDiff.Files) != 3 {
		test.Fatalf("parsed %d files", len(parsedDiff.Files))
	}
	// An unstyled theme means no ANSI of our own, so any ESC is the attacker's.
	for _, width := range []int{200, 40, 12} {
		for fileIndex, file := range parsedDiff.Files {
			model := New(theme.Theme{})
			model.SetFile(file)
			for step := 0; step < 12; step++ {
				assertSafe(test, "file render", model.Render(width, 6))
				model.CursorDown(1)
				model.ScrollRight(3)
			}
			model.GotoBottom()
			assertSafe(test, "bottom", model.Render(width, 6))
			_ = fileIndex
		}
	}
	model := New(theme.Theme{})
	model.SetDiffError(errors.New("bad \x1b[2J \u009b and \x07"))
	assertSafe(test, "error message", model.Render(80, 4))
	model.SetMessage("hello\x1b]0;x\x07")
	assertSafe(test, "message", model.Render(80, 4))
	if out := model.Render(80, 4); !strings.Contains(out, "hello") {
		test.Errorf("message lost: %q", out)
	}
}

func TestInvalidUTF8(test *testing.T) {
	got, width := displayText("caf\xe9 \x9b[2J")
	if got != "caf· ·[2J" || width != 9 {
		test.Errorf("displayText = %q, %d", got, width)
	}
	hunk := diff.Hunk{OldStart: 1, NewStart: 1, NewLines: 2, Lines: []diff.Line{
		{Kind: diff.Add, New: 1, Text: "caf\xe9"},
		{Kind: diff.Add, New: 2, Text: "\x9b[2J\xff\xfe end"},
	}}
	file := &diff.File{NewPath: "bad.txt", Status: diff.Added, Hunks: []diff.Hunk{hunk}}
	model := New(theme.New(true))
	model.SetFile(file)
	out := model.Render(40, 5)
	assertSafe(test, "styled", ansi.Strip(out))
	for i, text := range strings.Split(out, "\n") {
		if width := ansi.StringWidth(text); width != 40 {
			test.Errorf("row %d is %d cells wide, want 40", i, width)
		}
	}
}

func TestEmojiRowsAreExactlyPaneWidth(test *testing.T) {
	lines := []string{
		strings.Repeat("❤️", 30) + "x",
		"⚠️ warning ✔️ done " + strings.Repeat("⚠️", 5),
		"family 👨‍👩‍👧‍👦 and 👍🏽 flag 🇯🇵 end",
		"日本語 mixed ❤️ with ascii and \ttab",
		"é combining",
	}
	hunk := diff.Hunk{OldStart: 1, NewStart: 1}
	for i, text := range lines {
		hunk.Lines = append(hunk.Lines, diff.Line{Kind: diff.Add, New: i + 1, Text: text})
	}
	hunk.NewLines = len(lines)
	file := &diff.File{NewPath: "e.txt", Status: diff.Added, Hunks: []diff.Hunk{hunk}}

	const width = 40
	model := New(theme.New(true))
	model.SetFile(file)
	for off := 0; off <= 120; off++ {
		model.SetSize(width, 8)
		model.ScrollLeft(1000)
		model.ScrollRight(off)
		rows := strings.Split(model.Render(width, 8), "\n")
		if len(rows) != len(lines)+1 {
			test.Fatalf("rows = %d", len(rows))
		}
		for i, text := range rows {
			if got := ansi.StringWidth(text); got != width {
				test.Fatalf("xoff %d row %d is %d cells wide, want %d: %q", model.HorizontalOffset(), i, got, width, ansi.Strip(text))
			}
		}
	}
}

func TestSetFileKeepsPositionAcrossReparse(test *testing.T) {
	bytes, err := os.ReadFile(filepath.Join("testdata", "sample.diff"))
	if err != nil {
		test.Fatal(err)
	}
	firstDiff, _ := diff.Parse(string(bytes))
	secondDiff, _ := diff.Parse(string(bytes)) // a refresh: same content, new pointers
	model := New(theme.New(true))
	model.SetFile(firstDiff.File("server.go"))
	model.SetSize(60, 6)
	model.CursorDown(15)
	model.ScrollRight(5)
	cursor, top, horizontalOffset := model.window.Cursor(), model.window.Top(), model.window.HorizontalOffset()

	model.SetFile(secondDiff.File("server.go"))
	if model.window.Cursor() != cursor || model.window.Top() != top || model.window.HorizontalOffset() != horizontalOffset {
		test.Errorf("position after reparse = %d/%d/%d, want %d/%d/%d", model.window.Cursor(), model.window.Top(), model.window.HorizontalOffset(), cursor, top, horizontalOffset)
	}
	if got, ok := model.Cursor(); !ok || got.Old != 32 {
		test.Errorf("cursor line after reparse: %+v %v", got, ok)
	}

	// The refreshed file shrank: position clamps to the new bounds.
	small := *secondDiff.File("server.go")
	small.Hunks = small.Hunks[:1]
	model.SetFile(&small)
	if model.window.Cursor() >= len(model.rows) || model.window.Top() > model.window.MaximumTop() || model.window.Cursor() < model.window.Top() || model.window.Cursor() >= model.window.Top()+6 {
		test.Errorf("not clamped: cursor %d top %d rows %d", model.window.Cursor(), model.window.Top(), len(model.rows))
	}

	// A different path resets.
	model.SetFile(secondDiff.File("pkg/new.go"))
	model.SetFile(firstDiff.File("server.go"))
	if model.window.Cursor() != 0 || model.window.Top() != 0 || model.window.HorizontalOffset() != 0 {
		test.Errorf("different path should reset, got %d/%d/%d", model.window.Cursor(), model.window.Top(), model.window.HorizontalOffset())
	}
}
