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
	numClasses    = int(theme.NumSyntaxClasses)
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

// maxHighlightLen bounds the line length we are willing to lex; longer lines
// (minified bundles) render as plain text.
const maxHighlightLen = 2000

// ChromaHighlighter returns the default Highlighter, backed by Chroma. It
// remembers the lexer chosen for each path, so the (slow) file-name matching
// happens once per file.
func ChromaHighlighter() Highlighter {
	lexerFor := map[string]chroma.Lexer{}
	return func(path, text string) []Span {
		if text == "" || len(text) > maxHighlightLen {
			return plain(text)
		}
		lx, seen := lexerFor[path]
		if !seen {
			if lx = lexers.Match(path); lx != nil {
				lx = chroma.Coalesce(lx)
			}
			lexerFor[path] = lx
		}
		if lx == nil {
			return plain(text)
		}
		it, err := lx.Tokenise(nil, text)
		if err != nil {
			return plain(text)
		}
		var spans []Span
		for _, tok := range it.Tokens() {
			v := strings.TrimRight(tok.Value, "\n") // lexers may append a newline
			if v == "" {
				continue
			}
			spans = append(spans, Span{Text: v, Class: classify(tok.Type)})
		}
		if joined(spans) != text {
			return plain(text) // lexer altered the text; never show wrong code
		}
		return spans
	}
}

func joined(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func plain(text string) []Span {
	if text == "" {
		return nil
	}
	return []Span{{Text: text}}
}

// classify maps a token type to a class through the theme's mapping, the same
// one Markdown code blocks use, so both panes colour code identically.
func classify(t chroma.TokenType) Class { return theme.ClassOf(t) }
