package lexerguard

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"
)

// stringLines lexes text with the registered Java lexer and returns, for each
// line, the text of the tokens that are strings (not char literals).
func stringLines(test *testing.T, text string) []string {
	test.Helper()
	lexer := lexers.Get("java")
	if lexer == nil {
		test.Fatal("no Java lexer is registered")
	}
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		test.Fatalf("tokenise: %v", err)
	}
	result := make([]string, strings.Count(text, "\n")+1)
	line := 0
	for _, token := range iterator.Tokens() {
		for index, piece := range strings.Split(token.Value, "\n") {
			if index > 0 {
				line++
			}
			if line < len(result) && token.Type == chroma.LiteralString {
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
			name: "a complete text block spans its lines",
			text: "String s = \"\"\"\n    it's \"fine\"\n    more\\\"\n    \"\"\";\nint a = 1;",
			want: []string{"\"\"\"", "    it's \"fine\"", "    more\\\"", "    \"\"\"", ""},
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			if difference := cmp.Diff(testCase.want, stringLines(test, testCase.text)); difference != "" {
				test.Errorf("string text by line (-want +got):\n%s", difference)
			}
		})
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
