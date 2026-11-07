package diffview

import (
	"path"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// analysedLines and analysedBytes bound the text handed to content analysis:
// the first lines of the file, which is where a shebang or a package clause
// sits.
const (
	analysedLines = 10
	analysedBytes = 1024
)

// languageFromContent picks a lexer for a file whose name picked none, from
// the first lines of its text, or returns nil. lines must begin at the first
// line of the file. A first line that is a shebang decides alone, by the
// interpreter it names (Chroma's own analysis knows only the shell's, not
// Python's); any other first line sends the lines to Chroma's content analysis.
//
// Chroma's JSONata and Jungle lexers can run forever on some input (#265).
// Neither has an analyser, so analysis can't pick them, but a shebang naming
// them could; they are never chosen from content.
func languageFromContent(lines []string) chroma.Lexer {
	if len(lines) == 0 {
		return nil
	}
	var lexer chroma.Lexer
	if strings.HasPrefix(lines[0], "#!") {
		lexer = lexerFromShebang(lines[0])
	} else {
		lexer = lexers.Analyse(analysedText(lines))
	}
	if lexer == nil || neverFromContent(lexer) {
		return nil
	}
	return lexer
}

// analysedText joins the first lines of the file, within the byte bound.
func analysedText(lines []string) string {
	text := strings.Join(lines[:min(len(lines), analysedLines)], "\n")
	return text[:min(len(text), analysedBytes)]
}

// neverFromContent reports whether lexer is one that content must not pick.
func neverFromContent(lexer chroma.Lexer) bool {
	switch lexer.Config().Name {
	case "JSONata", "Jungle":
		return true
	}
	return false
}

// lexerFromShebang finds the lexer of the interpreter a "#!" line names:
// "#!/bin/sh", "#!/usr/bin/env python3", "#!/usr/bin/env -S node --flag".
// A version suffix ("python3.11") is dropped when the full name has no lexer.
func lexerFromShebang(first string) chroma.Lexer {
	fields := strings.Fields(strings.TrimPrefix(first, "#!"))
	if len(fields) == 0 {
		return nil
	}
	program := path.Base(fields[0])
	if program == "env" {
		program = ""
		for _, field := range fields[1:] {
			if !strings.HasPrefix(field, "-") && !strings.Contains(field, "=") {
				program = path.Base(field)
				break
			}
		}
	}
	if program == "" {
		return nil
	}
	if lexer := lexers.Get(program); lexer != nil {
		return lexer
	}
	return lexers.Get(strings.TrimRight(program, "0123456789."))
}
