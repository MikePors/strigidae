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

func (err *ParseError) Error() string {
	return fmt.Sprintf("parse diff: line %d: %s", err.Line, err.Msg)
}

var hunkRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

// Parse parses a git-format unified diff. Empty input yields an empty Diff and
// no error. The returned *Diff is never nil; when the error is non-nil it holds
// the best-effort parse.
func Parse(text string) (*Diff, error) {
	parsed := &parser{d: &Diff{}}
	n := 0
	for len(text) > 0 {
		var line string
		line, text, _ = strings.Cut(text, "\n")
		n++
		parsed.feed(n, line)
	}
	if parsed.inBody {
		parsed.fail(n, "truncated hunk")
	}
	if len(parsed.d.Files) == 0 && parsed.sawText && parsed.err == nil {
		parsed.err = &ParseError{Line: 1, Msg: "no diff found"}
	}
	return parsed.d, parsed.err
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

func (parsed *parser) fail(n int, message string) {
	if parsed.err == nil {
		parsed.err = &ParseError{Line: n, Msg: message}
	}
}

func (parser *parser) feed(n int, line string) {
	if strings.TrimSpace(line) != "" {
		parser.sawText = true
	}
	if parser.inBody {
		if parser.bodyLine(line) {
			return
		}
		parser.fail(n, "hunk ended before its line counts were reached")
		parser.inBody = false
	}
	// A no-newline marker may follow a hunk whose counts are exhausted.
	if parser.hasHunk && strings.HasPrefix(line, `\`) {
		parser.addLine(Line{Kind: NoNewline, Text: markerText(line)})
		return
	}
	if parser.hasHunk && line != "" && strings.IndexByte(" +-", line[0]) >= 0 {
		// Counts are used up but the diff goes on: the header lied.
		parser.fail(n, "diff line beyond the hunk's line counts")
		return
	}
	if rest, ok := strings.CutPrefix(line, "diff --git "); ok {
		parser.startFile(rest)
		return
	}
	if parser.file == nil {
		return // preamble or garbage before the first file
	}
	if m := hunkRegex.FindStringSubmatch(line); m != nil && parser.startHunk(n, m) {
		return
	}
	if !parser.hasHunk {
		parser.headerLine(line)
	}
}

// bodyLine consumes a hunk body line; false means the line does not belong to
// the hunk (malformed or truncated hunk).
func (parser *parser) bodyLine(line string) bool {
	switch {
	case strings.HasPrefix(line, `\`):
		parser.addLine(Line{Kind: NoNewline, Text: markerText(line)})
		return true
	case line == "" || line == "\r" || line[0] == ' ':
		if parser.oldRem == 0 || parser.newRem == 0 {
			return false
		}
		parser.oldRem--
		parser.newRem--
		parser.addLine(Line{Kind: Context, Old: parser.oldNo, New: parser.newNo, Text: content(line)})
		parser.oldNo++
		parser.newNo++
	case line[0] == '+':
		if parser.newRem == 0 {
			return false
		}
		parser.newRem--
		parser.addLine(Line{Kind: Add, New: parser.newNo, Text: content(line)})
		parser.newNo++
	case line[0] == '-':
		if parser.oldRem == 0 {
			return false
		}
		parser.oldRem--
		parser.addLine(Line{Kind: Delete, Old: parser.oldNo, Text: content(line)})
		parser.oldNo++
	default:
		return false
	}
	parser.inBody = parser.oldRem > 0 || parser.newRem > 0
	return true
}

func (parser *parser) addLine(l Line) {
	h := &parser.file.Hunks[len(parser.file.Hunks)-1]
	h.Lines = append(h.Lines, l)
}

func (parser *parser) startHunk(n int, m []string) bool {
	vals := [4]int{0, 1, 0, 1}
	for i, idx := range []int{1, 2, 3, 4} {
		if m[idx] == "" {
			continue
		}
		v, err := strconv.Atoi(m[idx])
		if err != nil || v > 1<<30 {
			parser.fail(n, "bad hunk header")
			return false
		}
		vals[i] = v
	}
	parser.file.Hunks = append(parser.file.Hunks, Hunk{
		OldStart: vals[0], OldLines: vals[1],
		NewStart: vals[2], NewLines: vals[3],
		Section: strings.TrimSpace(m[5]),
	})
	parser.hasHunk = true
	parser.oldRem, parser.newRem = vals[1], vals[3]
	parser.oldNo, parser.newNo = vals[0], vals[2]
	parser.inBody = parser.oldRem > 0 || parser.newRem > 0
	return true
}

func (parser *parser) startFile(rest string) {
	f := &File{}
	f.OldPath, f.NewPath = splitGitPaths(rest)
	parser.d.Files = append(parser.d.Files, f)
	parser.file, parser.hasHunk, parser.inBody = f, false, false
}

func (parser *parser) headerLine(line string) {
	file := parser.file
	switch {
	case strings.HasPrefix(line, "old mode "):
		file.OldMode = strings.TrimSpace(line[len("old mode "):])
	case strings.HasPrefix(line, "new mode "):
		file.NewMode = strings.TrimSpace(line[len("new mode "):])
	case strings.HasPrefix(line, "new file mode "):
		file.Status = Added
		file.NewMode = strings.TrimSpace(line[len("new file mode "):])
		file.OldPath = ""
	case strings.HasPrefix(line, "deleted file mode "):
		file.Status = Deleted
		file.OldMode = strings.TrimSpace(line[len("deleted file mode "):])
		file.NewPath = ""
	case strings.HasPrefix(line, "similarity index "):
		str := strings.TrimSuffix(strings.TrimSpace(line[len("similarity index "):]), "%")
		file.Similarity, _ = strconv.Atoi(str)
	case strings.HasPrefix(line, "rename from "):
		file.Status = Renamed
		file.OldPath = unquote(line[len("rename from "):])
	case strings.HasPrefix(line, "rename to "):
		file.Status = Renamed
		file.NewPath = unquote(line[len("rename to "):])
	case strings.HasPrefix(line, "copy from "):
		file.Status = Copied
		file.OldPath = unquote(line[len("copy from "):])
	case strings.HasPrefix(line, "copy to "):
		file.Status = Copied
		file.NewPath = unquote(line[len("copy to "):])
	case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
		file.Binary = true
	case strings.HasPrefix(line, "--- "):
		if path, ok := patchPath(line[4:]); ok && file.Status != Added {
			file.OldPath = path
		}
	case strings.HasPrefix(line, "+++ "):
		if path, ok := patchPath(line[4:]); ok && file.Status != Deleted {
			file.NewPath = path
		}
	}
}

// patchPath extracts the path from a "--- a/x" or "+++ b/x" operand. /dev/null
// reports ok=false.
func patchPath(str string) (string, bool) {
	if strings.HasPrefix(str, `"`) {
		quoted, err := strconv.QuotedPrefix(str) // git appends a TAB after names containing spaces
		if err != nil {
			return "", false // undecodable: keep the path from the diff --git line
		}
		str = quoted
	} else if i := strings.IndexByte(str, '\t'); i >= 0 {
		str = str[:i] // a tab-separated timestamp or trailing TAB
	}
	str = unquote(str)
	if str == "/dev/null" || str == "" {
		return "", false
	}
	if len(str) > 2 && (str[:2] == "a/" || str[:2] == "b/") {
		str = str[2:]
	}
	return str, true
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
	quoted, err := strconv.QuotedPrefix(rest)
	if err != nil {
		return "", "", false
	}
	first, err := strconv.Unquote(quoted)
	if err != nil {
		return "", "", false
	}
	second := unquote(strings.TrimPrefix(rest[len(quoted):], " "))
	return strings.TrimPrefix(first, "a/"), strings.TrimPrefix(second, "b/"), true
}

// unquote undoes git's C-style quoting of unusual paths.
func unquote(str string) string {
	str = strings.TrimRight(str, "\r")
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		if unquoted, err := strconv.Unquote(str); err == nil {
			return unquoted
		}
	}
	return str
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
