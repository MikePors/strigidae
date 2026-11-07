package diffview

import (
	"path"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// languageFromShebang picks a lexer for a file whose name picked none, from
// the first line of the file, or returns nil. Only a "#!" line counts, by the
// interpreter it names: "#!/bin/sh", "#!/usr/bin/env python3",
// "#!/usr/bin/env -S node --flag". Nothing else about the content is
// consulted.
//
// Chroma's JSONata and Jungle lexers can run forever on some input (#265).
// A shebang naming them would select one, so they are never chosen from
// content.
func languageFromShebang(firstLine string) chroma.Lexer {
	if !strings.HasPrefix(firstLine, "#!") {
		return nil
	}
	program := interpreterName(firstLine)
	if program == "" {
		return nil
	}
	lexer := lexerNamed(program)
	if lexer == nil {
		// A version suffix ("python3.11") is dropped when the full name has no lexer.
		lexer = lexerNamed(strings.TrimRight(program, "0123456789."))
	}
	if lexer == nil || neverFromContent(lexer) {
		return nil
	}
	return lexer
}

// interpreterName returns the program a "#!" line runs: the base name of its
// first word, or, for env, of the first word after it that is not a flag or a
// VAR=value assignment. A flag that takes an argument ("env -u NAME") is not
// understood: its argument is taken for the program.
func interpreterName(firstLine string) string {
	fields := strings.Fields(strings.TrimPrefix(firstLine, "#!"))
	if len(fields) == 0 {
		return ""
	}
	program := path.Base(fields[0])
	if program != "env" {
		return program
	}
	for _, field := range fields[1:] {
		if !strings.HasPrefix(field, "-") && !strings.Contains(field, "=") {
			return path.Base(field)
		}
	}
	return ""
}

// lexerNamed returns the lexer whose name or alias is name (ignoring case), or
// nil. lexers.Get alone is not enough: when no name or alias matches it falls
// back to file-name patterns, so "ruby3.2" would match the man page pattern
// "*.2" and come back as Groff.
func lexerNamed(name string) chroma.Lexer {
	if name == "" {
		return nil
	}
	lexer := lexers.Get(name)
	if lexer == nil {
		return nil
	}
	config := lexer.Config()
	if strings.EqualFold(config.Name, name) {
		return lexer
	}
	for _, alias := range config.Aliases {
		if strings.EqualFold(alias, name) {
			return lexer
		}
	}
	return nil
}

// neverFromContent reports whether lexer is one that content must not pick.
func neverFromContent(lexer chroma.Lexer) bool {
	switch lexer.Config().Name {
	case "JSONata", "Jungle":
		return true
	}
	return false
}
