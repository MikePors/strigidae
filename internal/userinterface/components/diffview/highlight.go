package diffview

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// Class is a coarse token category (the theme's SyntaxClass). Highlighters
// report classes rather than colours, so cached results stay valid when the
// theme changes.
type Class = theme.SyntaxClass

// Token classes.
const (
	ClassPlain    = theme.ClassPlain
	ClassKeyword  = theme.ClassKeyword
	ClassString   = theme.ClassString
	ClassComment  = theme.ClassComment
	ClassNumber   = theme.ClassNumber
	ClassFunction = theme.ClassFunction
	ClassType     = theme.ClassType
	ClassOperator = theme.ClassOperator
	ClassVariable = theme.ClassVariable
	ClassEscape   = theme.ClassEscape
	numberClasses = int(theme.NumberSyntaxClasses)
)

// Span is a run of text with one token class.
type Span struct {
	Text  string
	Class Class
}

// Highlighter splits one line of code into classed spans. path selects the
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
			spans = append(spans, Span{Text: value, Class: classify(token.Type)})
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
	return []Span{{Text: text}}
}

// classify maps a token type to a class through the theme's mapping, the same
// one Markdown code blocks use, so both panes colour code identically.
func classify(tokenType chroma.TokenType) Class { return theme.ClassOf(tokenType) }
