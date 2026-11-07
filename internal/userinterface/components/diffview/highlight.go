package diffview

import (
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Span is a run of text with one Chroma token type. Highlighters report
// token types rather than colours, so cached results stay valid when the
// theme changes; the theme gives each type its style.
type Span struct {
	Text  string
	Token chroma.TokenType
}

// Highlighter splits a block of code lines into typed spans, one result per
// line. path selects the language by file name. firstLine is the first line of
// the file the block belongs to (on the same side of the change), or "" when
// it is not known; when the name picks no language, a shebang in firstLine
// does. It is passed with every block of that side, whether or not the block
// begins the file: it comes from the file, not from the block.
// For every line the concatenated span text must equal the line.
//
// The lines are lexed together, so a construct that opens and closes inside
// the block (a block comment, a docstring, a raw string) is coloured on every
// line it covers. Anything that opened above the block is not known and is
// coloured as code, until spans of the whole file arrive (HighlightFile,
// Model.SetFileSpans).
type Highlighter func(path, firstLine string, lines []string) [][]Span

// maximumHighlightLength bounds the line length we are willing to lex; longer lines
// (minified bundles) render as plain text, and a block holding one is lexed
// line by line.
const maximumHighlightLength = 2000

// maximumBlockSize bounds the text of one block lexed in one piece, in bytes;
// a larger block is lexed line by line, as it is drawn. Lexing costs about
// 30 microseconds a line (BenchmarkHighlightHunkSide), so the whole block
// is a pause before the first row of its hunk appears: 128 KiB, about 2800
// lines, keeps that to under 100 ms for a side, twice that for both.
const maximumBlockSize = 128 * 1024

// blockBudget bounds the time one block may take to lex. Some input makes
// block lexing slow: in the Go and JavaScript lexers every unclosed "/*" scans
// to the end of the block, so a block of such lines is quadratic (128 KiB of
// them took 16 s). Chroma lexes as its token iterator is read, so lexing stops
// when the budget runs out and the rest of the block is lexed line by line,
// as a block over maximumBlockSize is. That is linear in the number of lines
// but still quadratic within each: long hostile lines (up to
// maximumHighlightLength) can still take seconds. A full block of ordinary code
// takes about 75 ms (BenchmarkHighlightHunkSide), so the budget leaves it
// whole.
const blockBudget = 100 * time.Millisecond

// tokensPerClockRead is how many tokens pass between readings of the clock:
// reading it for every token costs ordinary code about 3%. The tokens are
// counted as the lexer makes them, before Chroma's Coalesce merges a run of
// one type into one token, so a run of slow tokens of one type cannot hide
// from the clock.
const tokensPerClockRead = 16

// ChromaHighlighter returns the default Highlighter, backed by Chroma. It
// remembers the lexer chosen for each path, so the (slow) file-name matching
// happens once per file, and the lexer chosen for each first line, so a
// shebang is looked up once. Both depend only on their key, so nothing it
// remembers can go stale when a file is replaced. A file whose name picks no
// lexer is coloured from the shebang in firstLine, and is plain without one.
func ChromaHighlighter() Highlighter {
	lexerFor := map[string]chroma.Lexer{}
	lexerForFirstLine := map[string]chroma.Lexer{}
	return func(path, firstLine string, lines []string) [][]Span {
		lexer, seen := lexerFor[path]
		if !seen {
			lexer = lexerForPath(path)
			lexerFor[strings.Clone(path)] = lexer
		}
		if lexer == nil && strings.HasPrefix(firstLine, "#!") {
			lexer, seen = lexerForFirstLine[firstLine]
			if !seen {
				lexer = languageFromShebang(firstLine)
				// The line is a slice of the whole diff text; a copy keeps
				// the key from holding that text alive.
				lexerForFirstLine[strings.Clone(firstLine)] = lexer
			}
		}
		result := make([][]Span, len(lines))
		if lexer == nil {
			for i, line := range lines {
				result[i] = plain(line)
			}
			return result
		}
		if !fitsBlock(lines) {
			for i, line := range lines {
				result[i] = lexLine(lexer, line)
			}
			return result
		}
		return lexBlock(lexer, lines, time.Now)
	}
}

// lexerForPath returns the lexer the file name picks, as Chroma registers it,
// or nil when it picks none or a broken one.
func lexerForPath(path string) chroma.Lexer {
	if lexer := lexers.Match(path); lexer != nil && !broken(lexer) {
		return lexer
	}
	return nil
}

// broken reports whether lexer is one known to hang or panic on some input,
// and so is never chosen by file name or shebang. A lexer that spins without
// returning a token cannot be stopped from Go, and a panic on the UI goroutine
// ends strig. Lexers that pick a sub-lexer by name from Chroma's registry (a
// Markdown code fence) still reach these, as does Glamour's rendering of
// Markdown (#277).
// TestNoRegisteredLexerIsBroken finds them; drop an entry once a Chroma
// version that fixes it is pinned.
func broken(lexer chroma.Lexer) bool {
	switch lexer.Config().Name {
	case "JSONata", "Jungle": // loop forever on "/* a"
		return true
	case "YANG": // panics on a "/*" inside a block comment: its rules push a state that does not exist
		return true
	}
	return false
}

// fitsBlock reports whether lines may be lexed as one text.
func fitsBlock(lines []string) bool {
	size := 0
	for _, line := range lines {
		if len(line) > maximumHighlightLength {
			return false
		}
		size += len(line) + 1
	}
	return size <= maximumBlockSize
}

// lexBlock lexes lines as one text with lexer, as Chroma registers it (not
// wrapped in Coalesce: lexBlock does that itself, above the budget), and splits
// the tokens back into lines. A
// line whose spans do not join back to its text is plain. When lexing takes
// longer than blockBudget by the clock now, it stops, and the lines from the
// one it stopped in on are lexed one at a time.
func lexBlock(lexer chroma.Lexer, lines []string, now func() time.Time) [][]Span {
	result, finished, err := tokenLines(lexer, lines, now, blockBudget)
	if err != nil {
		result = make([][]Span, len(lines))
		for i, line := range lines {
			result[i] = plain(line)
		}
		return result
	}
	for i := finished; i < len(lines); i++ {
		result[i] = lexLine(lexer, lines[i])
	}
	for i, line := range lines[:finished] {
		if joined(result[i]) != line {
			result[i] = plain(line) // lexer altered the text; never show wrong code
		}
	}
	return result
}

// tokenLines lexes lines as one text with lexer, as Chroma registers it, and
// splits the tokens back into lines, without checking that each line's spans
// join back to its text. Lexing stops once budget has passed by the clock now;
// finished is the number of lines lexed whole, len(lines) unless it stopped.
func tokenLines(lexer chroma.Lexer, lines []string, now func() time.Time, budget time.Duration) (result [][]Span, finished int, err error) {
	budgeted := &budgetedLexer{Lexer: lexer, now: now, budget: budget}
	item, err := chroma.Coalesce(budgeted).Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return nil, 0, err
	}
	result = make([][]Span, len(lines))
	current := 0
	for token := item(); token != chroma.EOF; token = item() {
		tokenType := standard(token.Type)
		for index, piece := range strings.Split(token.Value, "\n") {
			if index > 0 {
				current++
			}
			if current >= len(lines) {
				break // the newline a lexer appends at the end
			}
			if piece != "" {
				result[current] = append(result[current], Span{Text: piece, Token: tokenType})
			}
		}
	}
	if budgeted.expired {
		return result, min(current, len(lines)), nil // the current line may be only partly lexed
	}
	return result, len(lines), nil
}

// budgetedLexer ends its tokens early, as if the text ended, once its
// budget has passed by the clock now since Tokenise, and then reports expired.
type budgetedLexer struct {
	chroma.Lexer
	now     func() time.Time
	budget  time.Duration
	expired bool
}

// Tokenise lexes text like the wrapped lexer, until the budget runs out.
func (lexer *budgetedLexer) Tokenise(options *chroma.TokeniseOptions, text string) (chroma.Iterator, error) {
	inner, err := lexer.Lexer.Tokenise(options, text)
	if err != nil {
		return nil, err
	}
	deadline := lexer.now().Add(lexer.budget)
	count := 0
	return func() chroma.Token {
		if lexer.expired {
			return chroma.EOF
		}
		count++
		if count%tokensPerClockRead == 0 && lexer.now().After(deadline) {
			lexer.expired = true
			return chroma.EOF
		}
		return inner()
	}, nil
}

// lexLine lexes one line on its own. Like lexBlock, it takes the lexer as
// Chroma registers it and merges runs of one token type itself.
func lexLine(lexer chroma.Lexer, text string) []Span {
	if text == "" || len(text) > maximumHighlightLength {
		return plain(text)
	}
	item, err := chroma.Coalesce(lexer).Tokenise(nil, text)
	if err != nil {
		return plain(text)
	}
	var spans []Span
	for _, token := range item.Tokens() {
		value := strings.TrimRight(token.Value, "\n") // lexers may append a newline
		if value == "" {
			continue
		}
		spans = append(spans, Span{Text: value, Token: standard(token.Type)})
	}
	if joined(spans) != text {
		return plain(text)
	}
	return spans
}

func joined(spans []Span) string {
	var builder strings.Builder
	for _, span := range spans {
		builder.WriteString(span.Text)
	}
	return builder.String()
}

func plain(text string) []Span {
	if text == "" {
		return nil
	}
	return []Span{{Text: text, Token: chroma.Text}}
}

// standard returns tokenType if it is a standard Chroma token type, or else
// the nearest standard one it inherits from, so every span has a style.
// Error is plain: lexing one line at a time, a lexer reports ordinary text as
// an error (an apostrophe in a block comment's middle line, a raw string's
// closing backtick), and colouring it would scatter error colour through
// comments.
func standard(tokenType chroma.TokenType) chroma.TokenType {
	if tokenType == chroma.Error {
		return chroma.Text
	}
	for _, key := range []chroma.TokenType{tokenType, tokenType.SubCategory(), tokenType.Category()} {
		if _, ok := chroma.StandardTypes[key]; ok {
			return key
		}
	}
	return chroma.Text
}
