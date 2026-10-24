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

// Highlighter splits one line of code into typed spans. path selects the
// language (by file name). The concatenated span text must equal text.
//
// Lines are highlighted independently, so constructs spanning lines (block
// comments, raw strings) are only coloured correctly on the line that opens
// them. That is the price of never lexing the whole file.
type Highlighter func(path, text string) []Span

// maximumHighlightLength bounds the line length we are willing to lex; longer lines
// (minified bundles) render as plain text.
const maximumHighlightLength = 2000

// ChromaHighlighter returns the default Highlighter, backed by Chroma. It
// remembers the lexer chosen for each path, so the (slow) file-name matching
// happens once per file.
func ChromaHighlighter() Highlighter {
	lexerFor := map[string]chroma.Lexer{}
	return func(path, text string) []Span {
		if text == "" || len(text) > maximumHighlightLength {
			return plain(text)
		}
		lexer, seen := lexerFor[path]
		if !seen {
			if lexer = lexers.Match(path); lexer != nil {
				lexer = chroma.Coalesce(lexer)
			}
			lexerFor[path] = lexer
		}
		if lexer == nil {
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
			return plain(text) // lexer altered the text; never show wrong code
		}
		return spans
	}
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
