package lexerguard

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"
)

// hostileText is what makes the real lexers hang or panic.
var hostileText = []string{"/* a", "/* a /* b */", "/* a /* b"}

// exampleFileNames maps each listed lexer to a file name its original filename
// globs matched.
var exampleFileNames = map[string]string{
	"JSONata": "x.jsonata",
	"Jungle":  "x.jungle",
	"YANG":    "x.yang",
}

// TestListedLexersWereRegistered: init replaced every listed name, so a
// rename in Chroma cannot leave one on the list that matches nothing.
func TestListedLexersWereRegistered(test *testing.T) {
	names := slices.Sorted(maps.Keys(replaced))
	if difference := cmp.Diff(slices.Sorted(slices.Values(Names())), names); difference != "" {
		test.Errorf("replaced names (-listed +replaced):\n%s", difference)
	}
	if len(Names()) != len(exampleFileNames) {
		test.Fatalf("Names() = %v, want the entries of the test's table", Names())
	}
	for _, name := range Names() {
		if !IsBroken(name) {
			test.Errorf("IsBroken(%q) = false", name)
		}
	}
}

func TestListedLexersArePlainText(test *testing.T) {
	for _, name := range Names() {
		fileName, ok := exampleFileNames[name]
		if !ok {
			test.Fatalf("no example file name for %q", name)
		}
		lookups := map[string]chroma.Lexer{
			"name":         lexers.Get(name),
			"lower case":   lexers.Get(strings.ToLower(name)),
			"match":        lexers.Match(fileName),
			"extension":    lexers.Get(strings.TrimPrefix(fileName, "x.")),
			"registry":     lexers.GlobalLexerRegistry.Get(name),
			"markdown tag": lexers.GlobalLexerRegistry.Get(strings.ToUpper(name)),
		}
		if len(lexers.Get(name).Config().Aliases) == 0 {
			test.Errorf("%q has no aliases; the replacement should copy them", name)
		}
		for _, alias := range lexers.Get(name).Config().Aliases {
			lookups["alias "+alias] = lexers.Get(alias)
		}
		for how, lexer := range lookups {
			test.Run(name+"/"+how, func(test *testing.T) {
				checkPlain(test, name, lexer)
			})
		}
	}
}

// checkPlain fails unless lexer is the plain replacement called name. It
// checks that lexer is the one init registered before lexing with it: the real
// Jungle lexer hangs on any text, so without the replacement the test would
// hang rather than fail.
func checkPlain(test *testing.T, name string, lexer chroma.Lexer) {
	test.Helper()
	if lexer == nil || lexer.Config().Name != name {
		test.Fatalf("lexer = %v, want one called %q", lexer, name)
	}
	if lexer != replaced[name] {
		test.Fatalf("lexer %q is not the replacement init registered", name)
	}
	if lexer.AnalyseText("/* a /* b */") != 0 {
		test.Errorf("the replacement of %q claims text", name)
	}
	for _, text := range hostileText {
		iterator, err := lexer.Tokenise(nil, text)
		if err != nil {
			test.Fatalf("tokenise %q: %v", text, err)
		}
		tokens := iterator.Tokens()
		if len(tokens) == 0 {
			test.Errorf("no tokens for %q", text)
		}
		var joined strings.Builder
		for _, token := range tokens {
			if token.Type != chroma.Text {
				test.Fatalf("token %+v for %q is not plain text", token, text)
			}
			joined.WriteString(token.Value)
		}
		if joined.String() != text {
			test.Errorf("tokens join to %q, want %q", joined.String(), text)
		}
	}
}

func TestUnlistedNameIsNotRegistered(test *testing.T) {
	registry := chroma.NewLexerRegistry()
	replaceBrokenLexers(registry)
	if len(registry.Lexers) != 0 {
		test.Errorf("replacing in an empty registry registered %d lexers", len(registry.Lexers))
	}
}
