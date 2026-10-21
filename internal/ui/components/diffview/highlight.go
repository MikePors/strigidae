package diffview

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Class is a coarse token category. Highlighters report classes rather than
// colours, so cached results stay valid when the theme changes.
type Class int

// Token classes.
const (
	ClassPlain Class = iota
	ClassKeyword
	ClassString
	ClassComment
	ClassNumber
	ClassFunction
	ClassType
	ClassOperator
	numClasses
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

func classify(t chroma.TokenType) Class {
	switch {
	case t == chroma.KeywordType:
		return ClassType
	case t.InCategory(chroma.Keyword), t == chroma.OperatorWord:
		return ClassKeyword
	case t.InSubCategory(chroma.LiteralString):
		return ClassString
	case t.InCategory(chroma.Comment):
		return ClassComment
	case t.InSubCategory(chroma.LiteralNumber):
		return ClassNumber
	case t == chroma.NameFunction, t == chroma.NameFunctionMagic, t == chroma.NameBuiltin, t == chroma.NameDecorator:
		return ClassFunction
	case t == chroma.NameClass, t == chroma.NameNamespace, t == chroma.NameException:
		return ClassType
	case t.InCategory(chroma.Operator):
		return ClassOperator
	}
	return ClassPlain
}
