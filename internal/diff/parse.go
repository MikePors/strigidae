package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParseError describes the first malformed spot Parse found.
type ParseError struct {
	Line    int // 1-based line number in the input
	Message string
}

func (err *ParseError) Error() string {
	return fmt.Sprintf("parse diff: line %d: %s", err.Line, err.Message)
}

var hunkRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

// Parse parses a git-format unified diff. Empty input yields an empty Diff and
// no error. The returned *Diff is never nil; when the error is non-nil it holds
// the best-effort parse.
func Parse(text string) (*Diff, error) {
	parsed := &parser{diff: &Diff{}}
	count := 0
	for len(text) > 0 {
		var line string
		line, text, _ = strings.Cut(text, "\n")
		count++
		parsed.feed(count, line)
	}
	if parsed.inBody {
		parsed.fail(count, "truncated hunk")
	}
	if len(parsed.diff.Files) == 0 && parsed.sawText && parsed.err == nil {
		parsed.err = &ParseError{Line: 1, Message: "no diff found"}
	}
	return parsed.diff, parsed.err
}

type parser struct {
	diff         *Diff
	file         *File
	hasHunk      bool // the current file has at least one hunk
	inBody       bool // inside a hunk body: line counts not yet exhausted
	oldRemaining int
	newRemaining int
	oldNo        int
	newNo        int
	err          error
	sawText      bool
}

func (parsed *parser) fail(count int, message string) {
	if parsed.err == nil {
		parsed.err = &ParseError{Line: count, Message: message}
	}
}

func (parser *parser) feed(count int, line string) {
	if strings.TrimSpace(line) != "" {
		parser.sawText = true
	}
	if parser.inBody {
		if parser.bodyLine(line) {
			return
		}
		parser.fail(count, "hunk ended before its line counts were reached")
		parser.inBody = false
	}
	// A no-newline marker may follow a hunk whose counts are exhausted.
	if parser.hasHunk && strings.HasPrefix(line, `\`) {
		parser.addLine(Line{Kind: NoNewline, Text: markerText(line)})
		return
	}
	if parser.hasHunk && line != "" && strings.IndexByte(" +-", line[0]) >= 0 {
		// Counts are used up but the diff goes on: the header lied.
		parser.fail(count, "diff line beyond the hunk's line counts")
		return
	}
	if rest, ok := strings.CutPrefix(line, "diff --git "); ok {
		parser.startFile(rest)
		return
	}
	if parser.file == nil {
		return // preamble or garbage before the first file
	}
	if messages := hunkRegex.FindStringSubmatch(line); messages != nil && parser.startHunk(count, messages) {
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
		if parser.oldRemaining == 0 || parser.newRemaining == 0 {
			return false
		}
		parser.oldRemaining--
		parser.newRemaining--
		parser.addLine(Line{Kind: Context, Old: parser.oldNo, New: parser.newNo, Text: content(line)})
		parser.oldNo++
		parser.newNo++
	case line[0] == '+':
		if parser.newRemaining == 0 {
			return false
		}
		parser.newRemaining--
		parser.addLine(Line{Kind: Add, New: parser.newNo, Text: content(line)})
		parser.newNo++
	case line[0] == '-':
		if parser.oldRemaining == 0 {
			return false
		}
		parser.oldRemaining--
		parser.addLine(Line{Kind: Delete, Old: parser.oldNo, Text: content(line)})
		parser.oldNo++
	default:
		return false
	}
	parser.inBody = parser.oldRemaining > 0 || parser.newRemaining > 0
	return true
}

func (parser *parser) addLine(line Line) {
	hunk := &parser.file.Hunks[len(parser.file.Hunks)-1]
	hunk.Lines = append(hunk.Lines, line)
}

func (parser *parser) startHunk(count int, messages []string) bool {
	values := [4]int{0, 1, 0, 1}
	for i, index := range []int{1, 2, 3, 4} {
		if messages[index] == "" {
			continue
		}
		value, err := strconv.Atoi(messages[index])
		if err != nil || value > 1<<30 {
			parser.fail(count, "bad hunk header")
			return false
		}
		values[i] = value
	}
	parser.file.Hunks = append(parser.file.Hunks, Hunk{
		OldStart: values[0], OldLines: values[1],
		NewStart: values[2], NewLines: values[3],
		Section: strings.TrimSpace(messages[5]),
	})
	parser.hasHunk = true
	parser.oldRemaining, parser.newRemaining = values[1], values[3]
	parser.oldNo, parser.newNo = values[0], values[2]
	parser.inBody = parser.oldRemaining > 0 || parser.newRemaining > 0
	return true
}

func (parser *parser) startFile(rest string) {
	file := &File{}
	file.OldPath, file.NewPath = splitGitPaths(rest)
	parser.diff.Files = append(parser.diff.Files, file)
	parser.file, parser.hasHunk, parser.inBody = file, false, false
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
		text := strings.TrimSuffix(strings.TrimSpace(line[len("similarity index "):]), "%")
		file.Similarity, _ = strconv.Atoi(text)
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
func patchPath(text string) (string, bool) {
	if strings.HasPrefix(text, `"`) {
		quoted, err := strconv.QuotedPrefix(text) // git appends a TAB after names containing spaces
		if err != nil {
			return "", false // undecodable: keep the path from the diff --git line
		}
		text = quoted
	} else if i := strings.IndexByte(text, '\t'); i >= 0 {
		text = text[:i] // a tab-separated timestamp or trailing TAB
	}
	text = unquote(text)
	if text == "/dev/null" || text == "" {
		return "", false
	}
	if len(text) > 2 && (text[:2] == "a/" || text[:2] == "b/") {
		text = text[2:]
	}
	return text, true
}

// splitGitPaths splits the operands of "diff --git a/x b/y". Paths may contain
// spaces, so when the two sides are equal it splits at the exact middle;
// otherwise it falls back to the first " b/" (rename lines override it anyway).
func splitGitPaths(rest string) (oldPath, newPath string) {
	rest = strings.TrimRight(rest, "\r")
	if strings.HasPrefix(rest, `"`) {
		if first, text, ok := splitQuoted(rest); ok {
			return first, text
		}
	}
	if mid := (len(rest) - 1) / 2; len(rest)%2 == 1 && rest[mid] == ' ' {
		first, text := rest[:mid], rest[mid+1:]
		if strings.HasPrefix(first, "a/") && strings.HasPrefix(text, "b/") && first[2:] == text[2:] {
			return first[2:], text[2:]
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
func unquote(text string) string {
	text = strings.TrimRight(text, "\r")
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		if unquoted, err := strconv.Unquote(text); err == nil {
			return unquoted
		}
	}
	return text
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
