package lexerguard

import (
	"slices"

	"github.com/alecthomas/chroma/v2"
)

// javaName is the name Chroma registers its Java lexer under.
const javaName = "Java"

// textBlockOpener is the root rule of Chroma's Java lexer that starts a text
// block: """ and a newline. The language allows spaces, tabs and form feeds
// between them, which this rule does not, so confineJavaStrings widens it.
const textBlockOpener = `"""\n`

// javaStatePatterns are the patterns of the rules of the Java string states in
// the Chroma version this was written against. confineJavaStrings rewrites
// both states, so it refuses to run on any other shape rather than silently
// drop a change Chroma made to them. An empty pattern is an include.
var javaStatePatterns = map[string][]string{
	"string":           {`[^\\"]+`, `\\\\`, `\\"`, `\\`, `"`},
	"multiline_string": {`"""`, `"`, ``},
}

// confineJavaStrings replaces the Java lexer in registry with one whose
// double-quoted strings end at the end of their line, and returns it, or
// returns nil when registry holds no Java lexer or its rules are not the shape
// this expects (TestJavaLexerIsConfined fails then).
//
// Chroma's Java string state accepts newlines, so one `"` that does not open a
// string (the diff view lexes each hunk as a fragment, which can begin inside
// a Javadoc comment or a text block) colours everything below it as a string.
// An ordinary Java string cannot span lines, so ending it at the newline loses
// nothing. A text block ("""), which does span lines, keeps its own state and
// runs to its closing """.
//
// A fragment cannot tell a closing """ that ends its line from an opener, so
// it still reads one as an opener; only lexing the whole file (see diffview's
// HighlightFile) gets that right.
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
	for state, want := range javaStatePatterns {
		if !slices.Equal(patternsOf(rules[state]), want) {
			return nil
		}
	}
	confined := rules.Clone()
	opener := slices.IndexFunc(confined["root"], func(rule chroma.Rule) bool { return rule.Pattern == textBlockOpener })
	if opener < 0 {
		return nil
	}
	confined["root"][opener].Pattern = `"""[ \t\f]*\n`
	confined["string"] = []chroma.Rule{
		{Pattern: `[^\\"\n]+`, Type: chroma.LiteralString},
		{Pattern: `\\\\`, Type: chroma.LiteralString},
		{Pattern: `\\"`, Type: chroma.LiteralString},
		{Pattern: `\\`, Type: chroma.LiteralString},
		{Pattern: `"`, Type: chroma.LiteralString, Mutator: chroma.Pop(1)},
		// Chroma ends a state on a newline that nothing matches anyway, as
		// error recovery; this rule says so, so the end of the string does
		// not rest on that fallback.
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
	replacement := chroma.MustNewLexer(&configuration, func() chroma.Rules { return confined })
	replacement.SetAnalyser(original.AnalyseText)
	return registry.Register(replacement)
}

// patternsOf returns the pattern of each of rules.
func patternsOf(rules []chroma.Rule) []string {
	patterns := make([]string, len(rules))
	for i, rule := range rules {
		patterns[i] = rule.Pattern
	}
	return patterns
}
