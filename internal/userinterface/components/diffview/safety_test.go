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

func assertSafe(t *testing.T, label, out string) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Errorf("%s: output is not valid UTF-8: %q", label, out)
	}
	for _, r := range out {
		if r == '\n' {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			t.Errorf("%s: unsafe rune %U in %q", label, r, out)
			return
		}
	}
}

func TestHostileContentIsSanitised(t *testing.T) {
	d, _ := diff.Parse(hostileDiff)
	if len(d.Files) != 3 {
		t.Fatalf("parsed %d files", len(d.Files))
	}
	// An unstyled theme means no ANSI of our own, so any ESC is the attacker's.
	for _, w := range []int{200, 40, 12} {
		for fi, f := range d.Files {
			m := New(theme.Theme{})
			m.SetFile(f)
			for step := 0; step < 12; step++ {
				assertSafe(t, "file render", m.Render(w, 6))
				m.CursorDown(1)
				m.ScrollRight(3)
			}
			m.GotoBottom()
			assertSafe(t, "bottom", m.Render(w, 6))
			_ = fi
		}
	}
	m := New(theme.Theme{})
	m.SetDiffError(errors.New("bad \x1b[2J \u009b and \x07"))
	assertSafe(t, "error message", m.Render(80, 4))
	m.SetMessage("hello\x1b]0;x\x07")
	assertSafe(t, "message", m.Render(80, 4))
	if out := m.Render(80, 4); !strings.Contains(out, "hello") {
		t.Errorf("message lost: %q", out)
	}
}

func TestInvalidUTF8(t *testing.T) {
	got, w := displayText("caf\xe9 \x9b[2J")
	if got != "caf· ·[2J" || w != 9 {
		t.Errorf("displayText = %q, %d", got, w)
	}
	h := diff.Hunk{OldStart: 1, NewStart: 1, NewLines: 2, Lines: []diff.Line{
		{Kind: diff.Add, New: 1, Text: "caf\xe9"},
		{Kind: diff.Add, New: 2, Text: "\x9b[2J\xff\xfe end"},
	}}
	f := &diff.File{NewPath: "bad.txt", Status: diff.Added, Hunks: []diff.Hunk{h}}
	m := New(theme.New(true))
	m.SetFile(f)
	out := m.Render(40, 5)
	assertSafe(t, "styled", ansi.Strip(out))
	for i, r := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(r); w != 40 {
			t.Errorf("row %d is %d cells wide, want 40", i, w)
		}
	}
}

func TestEmojiRowsAreExactlyPaneWidth(t *testing.T) {
	lines := []string{
		strings.Repeat("❤️", 30) + "x",
		"⚠️ warning ✔️ done " + strings.Repeat("⚠️", 5),
		"family 👨‍👩‍👧‍👦 and 👍🏽 flag 🇯🇵 end",
		"日本語 mixed ❤️ with ascii and \ttab",
		"é combining",
	}
	h := diff.Hunk{OldStart: 1, NewStart: 1}
	for i, text := range lines {
		h.Lines = append(h.Lines, diff.Line{Kind: diff.Add, New: i + 1, Text: text})
	}
	h.NewLines = len(lines)
	f := &diff.File{NewPath: "e.txt", Status: diff.Added, Hunks: []diff.Hunk{h}}

	const w = 40
	m := New(theme.New(true))
	m.SetFile(f)
	for off := 0; off <= 120; off++ {
		m.SetSize(w, 8)
		m.ScrollLeft(1000)
		m.ScrollRight(off)
		rows := strings.Split(m.Render(w, 8), "\n")
		if len(rows) != len(lines)+1 {
			t.Fatalf("rows = %d", len(rows))
		}
		for i, r := range rows {
			if got := ansi.StringWidth(r); got != w {
				t.Fatalf("xoff %d row %d is %d cells wide, want %d: %q", m.HorizontalOffset(), i, got, w, ansi.Strip(r))
			}
		}
	}
}

func TestSetFileKeepsPositionAcrossReparse(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "sample.diff"))
	if err != nil {
		t.Fatal(err)
	}
	d1, _ := diff.Parse(string(b))
	d2, _ := diff.Parse(string(b)) // a refresh: same content, new pointers
	m := New(theme.New(true))
	m.SetFile(d1.File("server.go"))
	m.SetSize(60, 6)
	m.CursorDown(15)
	m.ScrollRight(5)
	cursor, top, xoff := m.win.Cursor(), m.win.Top(), m.win.HorizontalOffset()

	m.SetFile(d2.File("server.go"))
	if m.win.Cursor() != cursor || m.win.Top() != top || m.win.HorizontalOffset() != xoff {
		t.Errorf("position after reparse = %d/%d/%d, want %d/%d/%d", m.win.Cursor(), m.win.Top(), m.win.HorizontalOffset(), cursor, top, xoff)
	}
	if got, ok := m.Cursor(); !ok || got.Old != 32 {
		t.Errorf("cursor line after reparse: %+v %v", got, ok)
	}

	// The refreshed file shrank: position clamps to the new bounds.
	small := *d2.File("server.go")
	small.Hunks = small.Hunks[:1]
	m.SetFile(&small)
	if m.win.Cursor() >= len(m.rows) || m.win.Top() > m.win.MaxTop() || m.win.Cursor() < m.win.Top() || m.win.Cursor() >= m.win.Top()+6 {
		t.Errorf("not clamped: cursor %d top %d rows %d", m.win.Cursor(), m.win.Top(), len(m.rows))
	}

	// A different path resets.
	m.SetFile(d2.File("pkg/new.go"))
	m.SetFile(d1.File("server.go"))
	if m.win.Cursor() != 0 || m.win.Top() != 0 || m.win.HorizontalOffset() != 0 {
		t.Errorf("different path should reset, got %d/%d/%d", m.win.Cursor(), m.win.Top(), m.win.HorizontalOffset())
	}
}
