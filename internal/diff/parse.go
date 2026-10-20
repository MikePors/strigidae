package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParseError describes the first malformed spot Parse found.
type ParseError struct {
	Line int // 1-based line number in the input
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse diff: line %d: %s", e.Line, e.Msg)
}

var hunkRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

// Parse parses a git-format unified diff. Empty input yields an empty Diff and
// no error. The returned *Diff is never nil; when the error is non-nil it holds
// the best-effort parse.
func Parse(text string) (*Diff, error) {
	p := &parser{d: &Diff{}}
	n := 0
	for len(text) > 0 {
		var line string
		line, text, _ = strings.Cut(text, "\n")
		n++
		p.feed(n, line)
	}
	if p.inBody {
		p.fail(n, "truncated hunk")
	}
	if len(p.d.Files) == 0 && p.sawText && p.err == nil {
		p.err = &ParseError{Line: 1, Msg: "no diff found"}
	}
	return p.d, p.err
}

type parser struct {
	d       *Diff
	file    *File
	hasHunk bool // the current file has at least one hunk
	inBody  bool // inside a hunk body: line counts not yet exhausted
	oldRem  int
	newRem  int
	oldNo   int
	newNo   int
	err     error
	sawText bool
}

func (p *parser) fail(n int, msg string) {
	if p.err == nil {
		p.err = &ParseError{Line: n, Msg: msg}
	}
}

func (p *parser) feed(n int, line string) {
	if strings.TrimSpace(line) != "" {
		p.sawText = true
	}
	if p.inBody {
		if p.bodyLine(line) {
			return
		}
		p.fail(n, "hunk ended before its line counts were reached")
		p.inBody = false
	}
	// A no-newline marker may follow a hunk whose counts are exhausted.
	if p.hasHunk && strings.HasPrefix(line, `\`) {
		p.addLine(Line{Kind: NoNewline, Text: markerText(line)})
		return
	}
	if rest, ok := strings.CutPrefix(line, "diff --git "); ok {
		p.startFile(rest)
		return
	}
	if p.file == nil {
		return // preamble or garbage before the first file
	}
	if m := hunkRE.FindStringSubmatch(line); m != nil && p.startHunk(n, m) {
		return
	}
	if !p.hasHunk {
		p.headerLine(line)
	}
}

// bodyLine consumes a hunk body line; false means the line does not belong to
// the hunk (malformed or truncated hunk).
func (p *parser) bodyLine(line string) bool {
	switch {
	case strings.HasPrefix(line, `\`):
		p.addLine(Line{Kind: NoNewline, Text: markerText(line)})
		return true
	case line == "" || line[0] == ' ':
		if p.oldRem == 0 || p.newRem == 0 {
			return false
		}
		p.oldRem--
		p.newRem--
		p.addLine(Line{Kind: Context, Old: p.oldNo, New: p.newNo, Text: content(line)})
		p.oldNo++
		p.newNo++
	case line[0] == '+':
		if p.newRem == 0 {
			return false
		}
		p.newRem--
		p.addLine(Line{Kind: Add, New: p.newNo, Text: content(line)})
		p.newNo++
	case line[0] == '-':
		if p.oldRem == 0 {
			return false
		}
		p.oldRem--
		p.addLine(Line{Kind: Delete, Old: p.oldNo, Text: content(line)})
		p.oldNo++
	default:
		return false
	}
	p.inBody = p.oldRem > 0 || p.newRem > 0
	return true
}

func (p *parser) addLine(l Line) {
	h := &p.file.Hunks[len(p.file.Hunks)-1]
	h.Lines = append(h.Lines, l)
}

func (p *parser) startHunk(n int, m []string) bool {
	vals := [4]int{0, 1, 0, 1}
	for i, idx := range []int{1, 2, 3, 4} {
		if m[idx] == "" {
			continue
		}
		v, err := strconv.Atoi(m[idx])
		if err != nil || v > 1<<30 {
			p.fail(n, "bad hunk header")
			return false
		}
		vals[i] = v
	}
	p.file.Hunks = append(p.file.Hunks, Hunk{
		OldStart: vals[0], OldLines: vals[1],
		NewStart: vals[2], NewLines: vals[3],
		Section: strings.TrimSpace(m[5]),
	})
	p.hasHunk = true
	p.oldRem, p.newRem = vals[1], vals[3]
	p.oldNo, p.newNo = vals[0], vals[2]
	p.inBody = p.oldRem > 0 || p.newRem > 0
	return true
}

func (p *parser) startFile(rest string) {
	f := &File{}
	f.OldPath, f.NewPath = splitGitPaths(rest)
	p.d.Files = append(p.d.Files, f)
	p.file, p.hasHunk, p.inBody = f, false, false
}

func (p *parser) headerLine(line string) {
	f := p.file
	switch {
	case strings.HasPrefix(line, "old mode "):
		f.OldMode = strings.TrimSpace(line[len("old mode "):])
	case strings.HasPrefix(line, "new mode "):
		f.NewMode = strings.TrimSpace(line[len("new mode "):])
	case strings.HasPrefix(line, "new file mode "):
		f.Status = Added
		f.NewMode = strings.TrimSpace(line[len("new file mode "):])
		f.OldPath = ""
	case strings.HasPrefix(line, "deleted file mode "):
		f.Status = Deleted
		f.OldMode = strings.TrimSpace(line[len("deleted file mode "):])
		f.NewPath = ""
	case strings.HasPrefix(line, "similarity index "):
		s := strings.TrimSuffix(strings.TrimSpace(line[len("similarity index "):]), "%")
		f.Similarity, _ = strconv.Atoi(s)
	case strings.HasPrefix(line, "rename from "):
		f.Status = Renamed
		f.OldPath = unquote(line[len("rename from "):])
	case strings.HasPrefix(line, "rename to "):
		f.Status = Renamed
		f.NewPath = unquote(line[len("rename to "):])
	case strings.HasPrefix(line, "copy from "):
		f.Status = Copied
		f.OldPath = unquote(line[len("copy from "):])
	case strings.HasPrefix(line, "copy to "):
		f.Status = Copied
		f.NewPath = unquote(line[len("copy to "):])
	case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
		f.Binary = true
	case strings.HasPrefix(line, "--- "):
		if path, ok := patchPath(line[4:]); ok && f.Status != Added {
			f.OldPath = path
		}
	case strings.HasPrefix(line, "+++ "):
		if path, ok := patchPath(line[4:]); ok && f.Status != Deleted {
			f.NewPath = path
		}
	}
}

// patchPath extracts the path from a "--- a/x" or "+++ b/x" operand. /dev/null
// reports ok=false.
func patchPath(s string) (string, bool) {
	if i := strings.IndexByte(s, '\t'); i >= 0 && !strings.HasPrefix(s, `"`) {
		s = s[:i] // some tools append a timestamp after a tab
	}
	s = unquote(s)
	if s == "/dev/null" {
		return "", false
	}
	if len(s) > 2 && (s[:2] == "a/" || s[:2] == "b/") {
		s = s[2:]
	}
	return s, true
}

// splitGitPaths splits the operands of "diff --git a/x b/y". Paths may contain
// spaces, so when the two sides are equal it splits at the exact middle;
// otherwise it falls back to the first " b/" (rename lines override it anyway).
func splitGitPaths(rest string) (oldPath, newPath string) {
	rest = strings.TrimRight(rest, "\r")
	if strings.HasPrefix(rest, `"`) {
		if a, b, ok := splitQuoted(rest); ok {
			return a, b
		}
	}
	if mid := (len(rest) - 1) / 2; len(rest)%2 == 1 && rest[mid] == ' ' {
		a, b := rest[:mid], rest[mid+1:]
		if strings.HasPrefix(a, "a/") && strings.HasPrefix(b, "b/") && a[2:] == b[2:] {
			return a[2:], b[2:]
		}
	}
	if i := strings.Index(rest, " b/"); i >= 0 {
		return strings.TrimPrefix(unquote(rest[:i]), "a/"), rest[i+3:]
	}
	return "", ""
}

func splitQuoted(rest string) (oldPath, newPath string, ok bool) {
	q, err := strconv.QuotedPrefix(rest)
	if err != nil {
		return "", "", false
	}
	first, err := strconv.Unquote(q)
	if err != nil {
		return "", "", false
	}
	second := unquote(strings.TrimPrefix(rest[len(q):], " "))
	return strings.TrimPrefix(first, "a/"), strings.TrimPrefix(second, "b/"), true
}

// unquote undoes git's C-style quoting of unusual paths.
func unquote(s string) string {
	s = strings.TrimRight(s, "\r")
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}

func content(line string) string {
	if line == "" {
		return ""
	}
	return strings.TrimSuffix(line[1:], "\r")
}

func markerText(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, `\`))
}
