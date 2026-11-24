package lexerguard

import (
	"slices"

	"github.com/alecthomas/chroma/v2"
)

// javaName is the name Chroma registers its Java lexer under.
const javaName = "Java"

// textBlockOpener is the root rule of Chroma's Java lexer that starts a text
// block: """ and a newline. The language allows white space between them,
// which this rule does not, so confineJavaStrings widens it to spaces and
// tabs. A form feed, also allowed, never reaches the lexer: the diff shows it
// as a middle dot.
const textBlockOpener = `"""\n`

// javaFragmentOpener is the root rule of the fragment lexer that starts a text
// block: a """ with something other than white space before it on its line,
// then white space and a newline. A hunk can begin inside a text block, and
// its first """ is then the end of the block; a closing """ that ends its line
// looks just like an opener, but one that stands alone at the start of its
// line has nothing before it, and an opener almost always follows code
// (`String text = """`). The lookbehind stays on one line: it holds one
// character that is not white space and then spaces and tabs.
//
// It is a guess with two known misses, both read the other way: an opener
// alone on its line (after a line break that follows an `=` or a `(`) is read
// as a closer, and a closer that follows text on its line (`</div>"""`) is
// read as an opener. A whole file is lexed with the registered lexer, which
// has neither.
const javaFragmentOpener = `(?<=\S[ \t]*)"""[ \t]*\n`

// javaStatePatterns are the patterns of the rules of the Java string states in
// the Chroma version this was written against. confineJavaStrings rewrites
// both states, so it refuses to run when their patterns differ rather than
// silently drop a change Chroma made to them. Only the patterns are compared,
// not the token types or mutators of the rules. An empty pattern is an
// include.
var javaStatePatterns = map[string][]string{
	"string":           {`[^\\"]+`, `\\\\`, `\\"`, `\\`, `"`},
	"multiline_string": {`"""`, `"`, ``},
}

// confineJavaStrings replaces the Java lexer in registry with one whose
// double-quoted strings end at the end of their line, and returns it, along
// with a second one for the hunks of a diff (see javaFragmentOpener) that it
// does not register. It returns nil, nil when registry holds no Java lexer or
// its rules are not the shape this expects (TestJavaLexerIsConfined fails
// then).
//
// Chroma's Java string state accepts newlines, so one `"` that does not open a
// string (the diff view lexes each hunk as a fragment, which can begin inside
// a Javadoc comment or a text block) colours everything below it as a string.
// An ordinary Java string cannot span lines, so ending it at the newline loses
// nothing. A text block ("""), which does span lines, keeps its own state and
// runs to its closing """.
//
// A fragment cannot tell a closing """ that ends its line from an opener, so
// the registered lexer still reads one as an opener, which is right for a
// whole file (see diffview's HighlightFile). The fragment lexer is for the
// text of a hunk.
func confineJavaStrings(registry *chroma.LexerRegistry) (registered, fragment chroma.Lexer) {
	position := indexOf(registry, javaName)
	if position < 0 {
		return nil, nil
	}
	original, ok := registry.Lexers[position].(*chroma.RegexLexer)
	if !ok {
		return nil, nil
	}
	rules, err := original.Rules()
	if err != nil {
		return nil, nil
	}
	for state, want := range javaStatePatterns {
		if !slices.Equal(patternsOf(rules[state]), want) {
			return nil, nil
		}
	}
	confined := rules.Clone()
	opener := slices.IndexFunc(confined["root"], func(rule chroma.Rule) bool { return rule.Pattern == textBlockOpener })
	if opener < 0 {
		return nil, nil
	}
	confined["root"][opener].Pattern = `"""[ \t]*\n`
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
	fragmentRules := confined.Clone()
	fragmentRules["root"][opener].Pattern = javaFragmentOpener
	configuration := *original.Config()
	replacement := chroma.MustNewLexer(&configuration, func() chroma.Rules { return confined })
	replacement.SetAnalyser(original.AnalyseText)
	fragmentConfiguration := *original.Config()
	fragmentLexer := chroma.MustNewLexer(&fragmentConfiguration, func() chroma.Rules { return fragmentRules })
	return registry.Register(replacement), fragmentLexer
}

// patternsOf returns the pattern of each of rules.
func patternsOf(rules []chroma.Rule) []string {
	patterns := make([]string, len(rules))
	for i, rule := range rules {
		patterns[i] = rule.Pattern
	}
	return patterns
}
