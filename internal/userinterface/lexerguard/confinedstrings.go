package lexerguard

import (
	"github.com/alecthomas/chroma/v2"
)

// javaName is the name Chroma registers its Java lexer under.
const javaName = "Java"

// confineJavaStrings replaces the Java lexer in registry with one whose
// double-quoted strings end at the end of their line, and returns it, or
// returns nil when registry holds no Java lexer or its rules are not the shape
// this expects (TestJavaLexerIsConfined fails then).
//
// Chroma's Java string state accepts newlines, so one `"` that does not open a
// string (the diff view lexes each hunk as a fragment, which can begin inside
// a Javadoc comment or a text block) colours everything below it as a string.
// A Java string cannot span lines, so ending it at the newline loses nothing.
// A text block ("""), which does, keeps its own state and runs to its closing
// """.
func confineJavaStrings(registry *chroma.LexerRegistry) chroma.Lexer {
	position := indexOf(registry, javaName)
	if position < 0 {
		return nil
	}
	original, ok := registry.Lexers[position].(*chroma.RegexLexer)
	if !ok {
		return nil
	}
	rules, err := original.Rules()
	if err != nil {
		return nil
	}
	_, hasString := rules["string"]
	_, hasTextBlock := rules["multiline_string"]
	if !hasString || !hasTextBlock {
		return nil
	}
	confined := rules.Clone()
	confined["string"] = []chroma.Rule{
		{Pattern: `[^\\"\n]+`, Type: chroma.LiteralString},
		{Pattern: `\\\\`, Type: chroma.LiteralString},
		{Pattern: `\\"`, Type: chroma.LiteralString},
		{Pattern: `\\`, Type: chroma.LiteralString},
		{Pattern: `"`, Type: chroma.LiteralString, Mutator: chroma.Pop(1)},
		{Pattern: `\n`, Type: chroma.TextWhitespace, Mutator: chroma.Pop(1)},
	}
	confined["multiline_string"] = []chroma.Rule{
		{Pattern: `"""`, Type: chroma.LiteralString, Mutator: chroma.Pop(1)},
		{Pattern: `"`, Type: chroma.LiteralString},
		{Pattern: `[^\\"]+`, Type: chroma.LiteralString},
		{Pattern: `\\\\`, Type: chroma.LiteralString},
		{Pattern: `\\"`, Type: chroma.LiteralString},
		{Pattern: `\\`, Type: chroma.LiteralString},
	}
	configuration := *original.Config()
	return registry.Register(chroma.MustNewLexer(&configuration, func() chroma.Rules { return confined }))
}
