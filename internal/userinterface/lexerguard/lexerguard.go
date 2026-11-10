// Package lexerguard replaces the Chroma lexers that hang or panic on some
// input with plain text, once, when the package is initialised.
//
// Every path to a lexer in strig goes through Chroma's one global registry: a
// file name or shebang in the diff view, the sub-lexer a Markdown or Gemtext
// code fence names in the diff view, and Glamour's lookup of a fence language
// in pull request bodies and comments. Replacing the entries there covers all
// of them. A lexer that spins without returning a token cannot be stopped from
// Go, and a panic on the UI goroutine ends strig.
//
// The replacement happens in an init, which must run before any lexer lookup:
// that is why both diffview and components/markdown import this package.
// Nothing writes to the registry after startup.
package lexerguard

import (
	"slices"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// brokenLexers are the names of the lexers known to hang or panic, with the
// reason for each. TestNoRegisteredLexerIsBroken in diffview finds them; drop
// an entry once a Chroma version that fixes it is pinned.
var brokenLexers = []brokenLexer{
	{"JSONata", `loops forever on "/* a"`},
	{"Jungle", `loops forever on "/* a"`},
	{"YANG", `panics on a "/*" inside a block comment: its rules push a state that does not exist`},
}

// brokenLexer names a lexer and says why it is replaced.
type brokenLexer struct {
	name   string
	reason string
}

// replaced holds the lexer init registered for each name it found and
// replaced. Tests compare its names with the list, so a rename in Chroma fails
// them, and check that lookups return these very lexers before lexing with
// them: the real ones hang, so lexing is no way to tell them apart.
var replaced map[string]chroma.Lexer

func init() {
	replaced = replaceBrokenLexers(lexers.GlobalLexerRegistry)
}

// replaceBrokenLexers swaps each listed lexer that registry holds for a plain
// text lexer with the same lookup configuration. A listed name the registry
// does not hold is left alone: registering it would invent a lexer. It returns
// the lexers it registered, by name.
func replaceBrokenLexers(registry *chroma.LexerRegistry) map[string]chroma.Lexer {
	registered := map[string]chroma.Lexer{}
	for _, broken := range brokenLexers {
		position := indexOf(registry, broken.name)
		if position < 0 {
			continue
		}
		registered[broken.name] = registry.Register(plainLexer(registry.Lexers[position].Config()))
	}
	return registered
}

// indexOf returns the position of the lexer called name in registry, or -1.
func indexOf(registry *chroma.LexerRegistry, name string) int {
	return slices.IndexFunc(registry.Lexers, func(lexer chroma.Lexer) bool {
		return lexer != nil && lexer.Config().Name == name
	})
}

// plainLexer returns a lexer that reads everything as chroma.Text and is
// found by the same names, aliases, file names and MIME types as the lexer
// whose configuration is original. It has no analyser, so it never wins a
// lexers.Analyse.
func plainLexer(original *chroma.Config) chroma.Lexer {
	configuration := *original
	configuration.Analyse = nil
	return chroma.MustNewLexer(&configuration, lexers.PlaintextRules)
}

// IsBroken reports whether name is the name of a lexer known to hang or panic.
// The registry no longer holds the real lexer under that name; this is a
// backstop for callers that choose by name.
func IsBroken(name string) bool {
	return slices.ContainsFunc(brokenLexers, func(entry brokenLexer) bool {
		return entry.name == name
	})
}

// Names returns the names of the replaced lexers.
func Names() []string {
	names := make([]string, len(brokenLexers))
	for i, entry := range brokenLexers {
		names[i] = entry.name
	}
	return names
}
