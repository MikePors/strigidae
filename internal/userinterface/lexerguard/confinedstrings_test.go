package lexerguard

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"
)

// javaTokens lexes text with the registered Java lexer. It fails unless the
// tokens join back to text: a lexer that drops a character (Chroma drops a
// newline that no rule matches) shifts every later line.
func javaTokens(test *testing.T, text string) []chroma.Token {
	test.Helper()
	lexer := lexers.Get("java")
	if lexer == nil {
		test.Fatal("no Java lexer is registered")
	}
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		test.Fatalf("tokenise: %v", err)
	}
	tokens := iterator.Tokens()
	var joined strings.Builder
	for _, token := range tokens {
		joined.WriteString(token.Value)
	}
	if want := strings.TrimSuffix(joined.String(), "\n"); want != strings.TrimSuffix(text, "\n") {
		test.Fatalf("tokens join to %q, want %q", joined.String(), text)
	}
	return tokens
}

// tokenLines returns, for each line of text, the text of the tokens of
// tokenType.
func tokenLines(test *testing.T, text string, tokenType chroma.TokenType) []string {
	test.Helper()
	result := make([]string, strings.Count(text, "\n")+1)
	line := 0
	for _, token := range javaTokens(test, text) {
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
	if lexer := confineJavaStrings(registry); lexer != nil {
		test.Errorf("confineJavaStrings(empty) = %v, want nil", lexer)
	}
	if len(registry.Lexers) != 0 {
		test.Errorf("confining in an empty registry registered %d lexers", len(registry.Lexers))
	}
}
