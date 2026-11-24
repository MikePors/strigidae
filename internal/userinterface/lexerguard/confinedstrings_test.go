package lexerguard

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"
)

// lexerTokens lexes text with lexer, failing unless the tokens join back to it.
func lexerTokens(test *testing.T, lexer chroma.Lexer, text string) []chroma.Token {
	test.Helper()
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		test.Fatalf("tokenise: %v", err)
	}
	tokens := iterator.Tokens()
	var joined strings.Builder
	for _, token := range tokens {
		joined.WriteString(token.Value)
	}
	if joined.String() != text {
		test.Fatalf("tokens join to %q, want %q", joined.String(), text)
	}
	return tokens
}

// tokenLines returns, for each line of text, the text of the tokens of
// tokenType.
func tokenLines(test *testing.T, text string, tokenType chroma.TokenType) []string {
	test.Helper()
	lexer := lexers.Get("java")
	if lexer == nil {
		test.Fatal("no Java lexer is registered")
	}
	return lexerTokenLines(test, lexer, text, tokenType)
}

// lexerTokenLines is tokenLines for the tokens of lexer.
func lexerTokenLines(test *testing.T, lexer chroma.Lexer, text string, tokenType chroma.TokenType) []string {
	test.Helper()
	result := make([]string, strings.Count(text, "\n")+1)
	line := 0
	for _, token := range lexerTokens(test, lexer, text) {
		for index, piece := range strings.Split(token.Value, "\n") {
			if index > 0 {
				line++
			}
			if line < len(result) && token.Type == tokenType {
				result[line] += piece
			}
		}
	}
	return result
}

func TestJavaStringsEndAtTheEndOfTheLine(test *testing.T) {
	for _, testCase := range []struct {
		name string
		text string
		want []string // the string text on each line
	}{
		{
			name: "a lone quote in a Javadoc the hunk starts inside",
			text: "     * Returns the \"name of the user.\n     */\n    public String name() {\n        return name;\n    }",
			want: []string{"\"name of the user.", "", "", "", ""},
		},
		{
			name: "a hunk that starts inside a text block",
			text: "        some text\n        \"\"\";\n        int a = 1;\n        int b = 2;",
			want: []string{"", "\"\"\";", "", ""},
		},
		{
			name: "an ordinary string, an escaped quote and a char literal",
			text: "String a = \"x\\\"y\";\nchar c = '\"';\nint z = 1;",
			want: []string{"\"x\\\"y\"", "", ""},
		},
		{
			name: "an apostrophe in a string and in a comment",
			text: "String a = \"don't\"; // it's\nint z = 1;",
			want: []string{"\"don't\"", ""},
		},
		{
			name: "a text block whose opener is followed by spaces",
			text: "String s = \"\"\" \t\n    some text\n    \"\"\";\nint a = 1;",
			want: []string{"\"\"\" \t", "    some text", "    \"\"\"", ""},
		},
		{
			name: "a complete text block spans its lines",
			text: "String s = \"\"\"\n    it's \"fine\"\n    more\\\"\n    \"\"\";\nint a = 1;",
			want: []string{"\"\"\"", "    it's \"fine\"", "    more\\\"", "    \"\"\"", ""},
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			if difference := cmp.Diff(testCase.want, tokenLines(test, testCase.text, chroma.LiteralString)); difference != "" {
				test.Errorf("string text by line (-want +got):\n%s", difference)
			}
		})
	}
}

// TestJavaCharLiteralsAreStillChars: the string helper leaves char literals
// out, so they are checked on their own.
func TestJavaCharLiteralsAreStillChars(test *testing.T) {
	text := "char a = '\"';\nchar b = '\\'';\nchar c = '\\u0041';\nint z = 1;"
	want := []string{"'\"'", "'\\''", "'\\u0041'", ""}
	if difference := cmp.Diff(want, tokenLines(test, text, chroma.LiteralStringChar)); difference != "" {
		test.Errorf("char literal text by line (-want +got):\n%s", difference)
	}
}

// TestJavaLexerIsConfined: init replaced the Java lexer, and every lookup that
// finds Java finds the replacement. Chroma changing the lexer's states makes
// confineJavaStrings return nil, which fails here.
func TestJavaLexerIsConfined(test *testing.T) {
	if confinedJava == nil {
		test.Fatal("init did not replace the Java lexer")
	}
	lookups := map[string]chroma.Lexer{
		"name":       lexers.Get("Java"),
		"lower case": lexers.Get("java"),
		"match":      lexers.Match("Main.java"),
		"registry":   lexers.GlobalLexerRegistry.Get("java"),
	}
	for how, lexer := range lookups {
		if lexer != confinedJava {
			test.Errorf("lookup by %s found %v, not the replacement", how, lexer)
		}
	}
	if confinedJava.Config().Name != javaName {
		test.Errorf("name = %q, want %q", confinedJava.Config().Name, javaName)
	}
}

func TestConfineJavaStringsWithoutJava(test *testing.T) {
	registry := chroma.NewLexerRegistry()
	if registered, fragment := confineJavaStrings(registry); registered != nil || fragment != nil {
		test.Errorf("confineJavaStrings(empty) = %v, %v, want nil, nil", registered, fragment)
	}
	if len(registry.Lexers) != 0 {
		test.Errorf("confining in an empty registry registered %d lexers", len(registry.Lexers))
	}
}

// TestJavaFragmentReadsALineOfOnlyTripleQuotesAsACloser: a hunk can begin
// inside a text block, and its first """ is then the end of the block, though
// it ends its line like an opener. The fragment lexer takes a """ with only
// white space before it as a closer; the registered lexer, for whole files,
// takes it as an opener.
func TestJavaFragmentReadsALineOfOnlyTripleQuotesAsACloser(test *testing.T) {
	for _, testCase := range []struct {
		name         string
		text         string
		wantFragment []string // the string text on each line, in the fragment lexer
		wantWhole    []string // and in the registered lexer
	}{
		{
			name:         "a hunk starting inside a text block whose closer ends its line",
			text:         "        select *\n        \"\"\"\n        .formatted(name);\n        int a = 1;",
			wantFragment: []string{"", `"""`, "", ""},
			wantWhole:    []string{"", `"""`, "        .formatted(name);", "        int a = 1;"},
		},
		{
			name:         "an opener after code",
			text:         "String sql = \"\"\"\n    select *\n    \"\"\";\nint a = 1;",
			wantFragment: []string{`"""`, "    select *", `    """`, ""},
			wantWhole:    []string{`"""`, "    select *", `    """`, ""},
		},
		{
			name:         "an opener after code, then spaces and a tab",
			text:         "foo(a, \"\"\" \t\n    text\n    \"\"\");\nint a = 1;",
			wantFragment: []string{"\"\"\" \t", "    text", `    """`, ""},
			wantWhole:    []string{"\"\"\" \t", "    text", `    """`, ""},
		},
		{
			// The limit of the guess: an opener that starts its own line,
			// after a line break that follows an = or a (, looks like a closer.
			name:         "an opener alone on its line",
			text:         "String sql =\n    \"\"\"\n    select *\n    \"\"\";",
			wantFragment: []string{"", `"""`, "", `""";`},
			wantWhole:    []string{"", `"""`, "    select *", `    """`},
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			fragment := lexerTokenLines(test, fragmentJava, testCase.text, chroma.LiteralString)
			if diff := cmp.Diff(testCase.wantFragment, fragment); diff != "" {
				test.Errorf("fragment lexer, string text per line (-want +got):\n%s", diff)
			}
			whole := tokenLines(test, testCase.text, chroma.LiteralString)
			if diff := cmp.Diff(testCase.wantWhole, whole); diff != "" {
				test.Errorf("registered lexer, string text per line (-want +got):\n%s", diff)
			}
		})
	}
}

// TestJavaFragmentIsNotRegistered: the registry holds the lexer for whole
// files, whatever lookup is used; the fragment lexer is a separate one that
// is still called Java, so the checks that go by name treat it the same.
func TestJavaFragmentIsNotRegistered(test *testing.T) {
	if fragmentJava == nil || JavaFragment() != fragmentJava {
		test.Fatal("init did not build the fragment lexer, or JavaFragment does not return it")
	}
	if fragmentJava == confinedJava {
		test.Error("the fragment lexer is the registered one")
	}
	for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
		if lexer == fragmentJava {
			test.Error("the fragment lexer is in the registry")
		}
	}
	if fragmentJava.Config().Name != javaName {
		test.Errorf("name = %q, want %q", fragmentJava.Config().Name, javaName)
	}
}
