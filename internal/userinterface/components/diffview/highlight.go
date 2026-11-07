package diffview

import (
	"strings"

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
// begins the file, and is the file's own line, never one of the block's.
// For every line the concatenated span text must equal the line.
//
// The lines are lexed together, so a construct that opens and closes inside
// the block (a block comment, a docstring, a raw string) is coloured on every
// line it covers. Anything that opened above the block is not known and is
// coloured as code (#262 reads the whole file to fix that).
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
			if lexer = lexers.Match(path); lexer != nil {
				lexer = chroma.Coalesce(lexer)
			}
			lexerFor[path] = lexer
		}
		if lexer == nil && firstLine != "" {
			lexer, seen = lexerForFirstLine[firstLine]
			if !seen {
				if lexer = languageFromShebang(firstLine); lexer != nil {
					lexer = chroma.Coalesce(lexer)
				}
				lexerForFirstLine[firstLine] = lexer
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
		return lexBlock(lexer, lines)
	}
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

// lexBlock lexes lines as one text and splits the tokens back into lines. A
// line whose spans do not join back to its text is plain.
func lexBlock(lexer chroma.Lexer, lines []string) [][]Span {
	result := make([][]Span, len(lines))
	item, err := lexer.Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		for i, line := range lines {
			result[i] = plain(line)
		}
		return result
	}
	current := 0
	for _, token := range item.Tokens() {
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
	for i, line := range lines {
		if joined(result[i]) != line {
			result[i] = plain(line) // lexer altered the text; never show wrong code
		}
	}
	return result
}

// lexLine lexes one line on its own.
func lexLine(lexer chroma.Lexer, text string) []Span {
	if text == "" || len(text) > maximumHighlightLength {
		return plain(text)
	}
	item, err := lexer.Tokenise(nil, text)
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
