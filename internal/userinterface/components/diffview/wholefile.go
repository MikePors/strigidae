package diffview

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
)

// MaximumFileSize bounds the text HighlightFile lexes, in bytes. Lexing costs
// about 30 microseconds a line off the UI goroutine, so 1 MiB (some 25,000
// lines of ordinary code) takes about a second before its colours replace the
// per-hunk ones.
const MaximumFileSize = 1 << 20

// fileBudget bounds the time HighlightFile may lex one file. An ordinary file
// of MaximumFileSize takes about a second; hostile text can make a lexer
// quadratic (lines of unclosed "/*" in Go: 256 KiB took over a minute), and
// the budget stops it.
const fileBudget = 5 * time.Second

// Reasons HighlightFile gives no spans.
var (
	ErrFileTooLarge = errors.New("file too large to highlight whole")
	ErrFileBinary   = errors.New("binary file")
	ErrNoLanguage   = errors.New("no language for the file")
	ErrLexTimeout   = errors.New("lexing the whole file took too long")
	ErrLexPanic     = errors.New("the lexer panicked")
)

// HighlightFile lexes the whole text of one side of a file and returns the
// spans of each of its lines, index 0 being line 1. Each line is lexed as the
// diff rows show it (tabs expanded, controls replaced, a trailing carriage
// return dropped), so its spans join back to the text of its row. A line whose
// spans do not join back to its text is nil.
//
// The language is picked as the Highlighter picks it: by path, else by the
// shebang on the text's first line, never a broken lexer. Lexing that takes
// longer than fileBudget fails with ErrLexTimeout rather than give part of the
// file, so the rows keep their per-hunk colours. A lexer that panics
// (none should: lexerguard replaces the known ones) fails with ErrLexPanic
// rather than end strig, which a panic in a tea.Cmd would. It shares nothing with a
// Highlighter and touches neither the Chroma style registry nor Glamour, so it
// may run in a tea.Cmd.
func HighlightFile(path string, contents []byte) ([][]Span, error) {
	return highlightFile(path, contents, time.Now, lexerForFile)
}

// lexerForFile picks the lexer for a whole file as the Highlighter does: by
// path, else by the shebang on its first line. It returns nil for none.
func lexerForFile(path, firstLine string) chroma.Lexer {
	if lexer := lexerForPath(path); lexer != nil {
		return lexer
	}
	return languageFromShebang(firstLine)
}

// highlightFile is HighlightFile timed by the clock now, with the lexer
// picked by pickLexer. Tests pass their own pickLexer to reach a lexer that
// panics, since the registry no longer holds one.
func highlightFile(path string, contents []byte, now func() time.Time, pickLexer func(path, firstLine string) chroma.Lexer) (spans [][]Span, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			spans, err = nil, fmt.Errorf("%w: %v", ErrLexPanic, recovered)
		}
	}()
	if len(contents) > MaximumFileSize {
		return nil, fmt.Errorf("%w: %d bytes", ErrFileTooLarge, len(contents))
	}
	if bytes.IndexByte(contents, 0) >= 0 {
		return nil, ErrFileBinary
	}
	rawLines := strings.Split(string(contents), "\n")
	lines := make([]string, len(rawLines))
	for i, line := range rawLines {
		lines[i], _ = displayText(strings.TrimSuffix(line, "\r"))
	}
	lexer := pickLexer(path, lines[0])
	if lexer == nil {
		return nil, ErrNoLanguage
	}
	spans, finished, err := tokenLines(lexer, lines, now, fileBudget)
	if err != nil {
		return nil, fmt.Errorf("lex: %w", err)
	}
	if finished < len(lines) {
		return nil, fmt.Errorf("%w: stopped at line %d of %d", ErrLexTimeout, finished+1, len(lines))
	}
	for i, line := range lines {
		if joined(spans[i]) != line {
			spans[i] = nil
		}
	}
	return spans, nil
}

// wholeSide is the spans of a whole side of the shown file, from
// SetFileSpans.
type wholeSide struct {
	objectIdentifier string
	lines            [][]Span
}

// wholeRow is the whole-file spans of one row, worked out once.
type wholeRow struct {
	spans []Span
	ok    bool
}

// SetFileSpans gives the spans of every line of one side of the shown file
// (from HighlightFile), the side named by objectIdentifier as on the diff's
// index line. Rows are then coloured from the whole file, so a construct that
// opened above a hunk is known; a row whose text differs from its line keeps
// its per-hunk colours. Spans for another file, or for a side whose
// identifier differs from the shown file's, are ignored. Giving the same
// side again changes nothing.
func (model *Model) SetFileSpans(side github.DiffSide, objectIdentifier string, lines [][]Span) {
	if model.file == nil || objectIdentifier == "" {
		return
	}
	target, want := &model.newWhole, model.file.NewObjectIdentifier
	if side == github.SideLeft {
		target, want = &model.oldWhole, model.file.OldObjectIdentifier
	}
	if objectIdentifier != want || target.objectIdentifier == objectIdentifier {
		return
	}
	*target = wholeSide{objectIdentifier: objectIdentifier, lines: lines}
	model.wholeRows = map[int]wholeRow{}
}

// wholeSpans returns the whole-file spans of base row baseIndex, if a side
// covering it has arrived and its line matches the row's text. Context rows
// take the new side, or the old one when the new side does not match.
func (model *Model) wholeSpans(baseIndex int) ([]Span, bool) {
	if model.oldWhole.lines == nil && model.newWhole.lines == nil {
		return nil, false
	}
	if known, ok := model.wholeRows[baseIndex]; ok {
		return known.spans, known.ok
	}
	row := model.base[baseIndex]
	var spans []Span
	ok := false
	if row.line.Kind != diff.Delete {
		spans, ok = lineSpans(model.newWhole.lines, row.line.New, row.text)
	}
	if !ok && row.line.Kind != diff.Add {
		spans, ok = lineSpans(model.oldWhole.lines, row.line.Old, row.text)
	}
	model.wholeRows[baseIndex] = wholeRow{spans: spans, ok: ok}
	return spans, ok
}

// lineSpans returns the spans of 1-based line number in lines when they join
// back to text.
func lineSpans(lines [][]Span, number int, text string) ([]Span, bool) {
	if number < 1 || number > len(lines) {
		return nil, false
	}
	spans := lines[number-1]
	if joined(spans) != text {
		return nil, false
	}
	return spans, true
}
