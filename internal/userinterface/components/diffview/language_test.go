package diffview

import (
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// lexedAs returns lines as the named Chroma lexer lexes them as one block.
func lexedAs(test *testing.T, name string, lines []string) [][]Span {
	test.Helper()
	lexer := lexers.Get(name)
	if lexer == nil {
		test.Fatalf("no lexer called %q", name)
	}
	return lexBlock(chroma.Coalesce(lexer), lines, time.Now)
}

// allPlain returns lines as a Highlighter reports a file with no language.
func allPlain(lines []string) [][]Span {
	spans := make([][]Span, len(lines))
	for i, line := range lines {
		spans[i] = plain(line)
	}
	return spans
}

func TestLanguageFromShebang(test *testing.T) {
	pythonLines := []string{"#!/usr/bin/env python3", "import os", "", "def main():", "    return os.getcwd()  # here"}
	shellLines := []string{"#!/bin/sh", "set -e", `if [ -n "$HOME" ]; then echo "$HOME"; fi`}
	tests := []struct {
		name      string
		path      string
		firstLine string
		lines     []string
		language  string // the Chroma lexer expected; "" for plain
	}{
		{name: "python through env", path: "bin/release", firstLine: pythonLines[0], lines: pythonLines, language: "python"},
		{name: "shell", path: "bin/release", firstLine: shellLines[0], lines: shellLines, language: "bash"},
		{name: "interpreter with a version", path: "run", firstLine: "#!/usr/bin/python3.11", lines: []string{"#!/usr/bin/python3.11", "print(1)"}, language: "python"},
		{name: "python 2.7 is not a man page", path: "run", firstLine: "#!/usr/bin/python2.7", lines: []string{"print 1"}, language: "python"},
		{name: "ruby 3.2 is not a man page", path: "run", firstLine: "#!/usr/bin/env ruby3.2", lines: []string{"puts 1"}, language: "ruby"},
		{name: "lua 5.4 is not a man page", path: "run", firstLine: "#!/usr/bin/lua5.4", lines: []string{"print(1)"}, language: "lua"},
		{name: "env with a flag and a variable", path: "run", firstLine: "#!/usr/bin/env -S LANG=C ruby -w", lines: []string{"puts 1"}, language: "ruby"},
		{name: "spaces after the marker", path: "run", firstLine: "#! /usr/bin/perl", lines: []string{"print 1;"}, language: "perl"},
		{name: "a block that has no first line stays plain", path: "bin/release", firstLine: "", lines: pythonLines},
		{name: "a first line that is no shebang stays plain", path: "main", firstLine: "package main", lines: []string{"package main", "", "func main() {}"}},
		{name: "an unknown interpreter stays plain", path: "bin/release", firstLine: "#!/opt/odd/frobnicate", lines: []string{"x"}},
		{name: "a bare marker stays plain", path: "bin/release", firstLine: "#!", lines: []string{"x"}},
		{name: "env alone stays plain", path: "bin/release", firstLine: "#!/usr/bin/env", lines: []string{"x"}},
		{name: "a number is no interpreter", path: "bin/release", firstLine: "#!/usr/bin/env 1", lines: []string{"x"}},
		{name: "plain text stays plain", path: "NOTES", firstLine: "just words", lines: []string{"just words", "and more"}},
		{name: "a name match wins over a misleading shebang", path: "tool.py", firstLine: "#!/bin/sh", lines: pythonLines[1:], language: "python"},
		{name: "a lexer that can hang is never chosen from content", path: "bin/release", firstLine: "#!/usr/bin/env jsonata", lines: []string{"$x"}},
		{name: "the other lexer that can hang", path: "bin/release", firstLine: "#!/usr/bin/env jungle", lines: []string{"x"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			want := allPlain(testCase.lines)
			if testCase.language != "" {
				want = lexedAs(test, testCase.language, testCase.lines)
			}
			got := ChromaHighlighter()(testCase.path, testCase.firstLine, testCase.lines)
			if diff := cmp.Diff(want, got); diff != "" {
				test.Errorf("spans differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestShebangNamesTheExactLanguage proves an interpreter is found by Chroma's
// name or alias only: lexers.Get alone would answer Groff for a name ending in
// ".1" to ".9", a man page pattern.
func TestShebangNamesTheExactLanguage(test *testing.T) {
	tests := []struct {
		firstLine string
		want      string // Chroma's name for the language; "" for none
	}{
		{"#!/usr/bin/python2.7", "Python"},
		{"#!/usr/bin/env ruby3.2", "Ruby"},
		{"#!/usr/bin/lua5.4", "Lua"},
		{"#!/usr/bin/python3.11", "Python"},
		{"#!/usr/bin/env python3", "Python"},
		{"#!/bin/sh", "Bash"},
		{"#!/usr/bin/env 1", ""},
		{"#!/usr/bin/7", ""},
		{"#!/usr/bin/env 2.5", ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.firstLine, func(test *testing.T) {
			name := ""
			if lexer := languageFromShebang(testCase.firstLine); lexer != nil {
				name = lexer.Config().Name
			}
			if name != testCase.want {
				test.Errorf("language = %q, want %q", name, testCase.want)
			}
		})
	}
}

// TestNameMatchWinsOverShebang proves a file name that picks a lexer is not
// overridden by the first line: shell text in a .py file is lexed as Python.
func TestNameMatchWinsOverShebang(test *testing.T) {
	lines := []string{"#!/bin/sh", "def f(): pass"}
	got := ChromaHighlighter()("tool.py", lines[0], lines)
	if diff := cmp.Diff(lexedAs(test, "python", lines), got); diff != "" {
		test.Errorf("spans differ (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(lexedAs(test, "bash", lines), got); diff == "" {
		test.Error("the shebang was used although the name matched")
	}
}

// startingAt moves the one hunk of file to start at line start on both sides.
func startingAt(file *diff.File, start int) *diff.File {
	hunk := &file.Hunks[0]
	hunk.OldStart, hunk.NewStart = start, start
	for i := range hunk.Lines {
		if hunk.Lines[i].Old != 0 {
			hunk.Lines[i].Old += start - 1
		}
		if hunk.Lines[i].New != 0 {
			hunk.Lines[i].New += start - 1
		}
	}
	return file
}

// TestDrawnHunkUsesItsPositionInTheFile proves the drawn file passes whether
// a side starts at line 1: the same extensionless script is coloured as
// Python from a hunk at line 1 and plain from a hunk at line 40.
func TestDrawnHunkUsesItsPositionInTheFile(test *testing.T) {
	lines := []codeLine{
		{diff.Context, "#!/usr/bin/env python3"},
		{diff.Add, "def main():"},
		{diff.Add, "    return 1"},
		{diff.Context, "main()"},
	}
	texts := make([]string, len(lines))
	for i, line := range lines {
		texts[i] = line.text
	}
	tests := []struct {
		name  string
		start int
		want  [][]Span
	}{
		{"hunk at line 1", 1, lexedAs(test, "python", texts)},
		{"hunk at line 40", 40, allPlain(texts)},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got := rowSpans(test, startingAt(hunkFile("bin/release", lines), testCase.start))
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("spans differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDeletedFileUsesItsOldPath proves a deleted file, whose new path is
// empty, is lexed by the language of its old name and content.
func TestDeletedFileUsesItsOldPath(test *testing.T) {
	lines := []codeLine{{diff.Delete, "#!/bin/sh"}, {diff.Delete, "echo " + strings.Repeat("x", 3)}}
	file := hunkFile("bin/release", lines)
	file.NewPath = ""
	file.Hunks[0].NewStart, file.Hunks[0].NewLines = 0, 0
	got := rowSpans(test, file)
	if diff := cmp.Diff(lexedAs(test, "bash", []string{lines[0].text, lines[1].text}), got); diff != "" {
		test.Errorf("spans differ (-want +got):\n%s", diff)
	}
}

// texts returns the text of each of lines.
func texts(lines []codeLine) []string {
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = line.text
	}
	return result
}

// twoHunkFile builds a file called path with a hunk of first at line 1 and a
// hunk of second at line start.
func twoHunkFile(path string, first, second []codeLine, start int) *diff.File {
	file := hunkFile(path, first)
	file.Hunks = append(file.Hunks, startingAt(hunkFile(path, second), start).Hunks[0])
	return file
}

// hunkSpans returns the spans drawn for the code rows of hunk, in order.
func hunkSpans(model *Model, hunk int) [][]Span {
	var spans [][]Span
	for i, row := range model.rows {
		if row.hunk == hunk && row.line != nil {
			spans = append(spans, model.codeSpans(i))
		}
	}
	return spans
}

// TestSidesHaveTheirOwnLanguage proves the language of each side comes from
// that side's first line: a hunk at line 1 that swaps a shell shebang for a
// Python one draws the deleted lines as shell and the added lines as Python.
func TestSidesHaveTheirOwnLanguage(test *testing.T) {
	file := hunkFile("bin/release", []codeLine{
		{diff.Delete, "#!/bin/sh"},
		{diff.Delete, "echo old"},
		{diff.Add, "#!/usr/bin/env python3"},
		{diff.Add, "def main(): return 1"},
	})
	model := New(theme.New(true))
	model.SetFile(file)
	got := hunkSpans(model, 0)
	want := append(
		lexedAs(test, "bash", []string{"#!/bin/sh", "echo old"}),
		lexedAs(test, "python", []string{"#!/usr/bin/env python3", "def main(): return 1"})...,
	)
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("spans differ (-want +got):\n%s", diff)
	}
}

// TestLanguageDoesNotDependOnDrawingOrder proves a hunk below the top of the
// file is coloured by the file's first line even when it is drawn first, or
// when the hunk holding that line is hidden and never drawn.
func TestLanguageDoesNotDependOnDrawingOrder(test *testing.T) {
	first := []codeLine{{diff.Context, "#!/usr/bin/env python3"}, {diff.Add, "import os"}}
	second := []codeLine{{diff.Context, `"""doc`}, {diff.Add, `more"""`}}
	want := lexedAs(test, "python", texts(second))
	tests := []struct {
		name string
		draw func(model *Model) [][]Span // the spans the later hunk is drawn with
	}{
		{"drawn before the top hunk", func(model *Model) [][]Span {
			hunkSpans(model, 1)
			hunkSpans(model, 0)
			return hunkSpans(model, 1)
		}},
		{"drawn first, read again", func(model *Model) [][]Span {
			hunkSpans(model, 1)
			return hunkSpans(model, 1)
		}},
		{"top hunk hidden", func(model *Model) [][]Span {
			model.SetHunkHidden(func(file *diff.File, hunk int) bool { return file == model.file && hunk == 0 })
			return hunkSpans(model, 1)
		}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := New(theme.New(true))
			model.SetFile(twoHunkFile("bin/run", first, second, 100))
			if diff := cmp.Diff(want, testCase.draw(model)); diff != "" {
				test.Errorf("spans differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSetFileForgetsTheLanguage proves a file set again on the same pane (a
// reload after a force-push) gets the language of its own first line.
func TestSetFileForgetsTheLanguage(test *testing.T) {
	model := New(theme.New(true))
	model.SetFile(hunkFile("bin/run", []codeLine{{diff.Context, "#!/bin/sh"}, {diff.Add, "echo hi"}}))
	hunkSpans(model, 0)
	python := []codeLine{{diff.Context, "#!/usr/bin/env python3"}, {diff.Add, "def main(): return 1"}}
	model.SetFile(hunkFile("bin/run", python))
	if diff := cmp.Diff(lexedAs(test, "python", texts(python)), hunkSpans(model, 0)); diff != "" {
		test.Errorf("spans differ (-want +got):\n%s", diff)
	}
	model.SetFile(hunkFile("bin/run", []codeLine{{diff.Context, "plain words"}, {diff.Add, "def main(): return 1"}}))
	if got := hunkSpans(model, 0); !cmp.Equal(allPlain([]string{"plain words", "def main(): return 1"}), got) {
		test.Errorf("a file without a shebang kept an old language: %v", got)
	}
}

// TestFileWithoutShebangStaysPlain proves a file the name does not match is
// coloured only from a shebang: content that Chroma's analysis would call
// GDScript is plain.
func TestFileWithoutShebangStaysPlain(test *testing.T) {
	lines := []codeLine{{diff.Context, "package main"}, {diff.Context, ""}, {diff.Add, "func main() {}"}}
	got := rowSpans(test, hunkFile("bin/release", lines))
	if diff := cmp.Diff(allPlain(texts(lines)), got); diff != "" {
		test.Errorf("spans differ (-want +got):\n%s", diff)
	}
}

// TestFirstLines proves which first lines a file's hunks give.
func TestFirstLines(test *testing.T) {
	// hunk builds a one-hunk file whose hunk starts at the given line on each
	// side (0 on a side the file does not have).
	hunk := func(oldStart, newStart int, lines ...codeLine) *diff.File {
		file := hunkFile("run", lines)
		file.Hunks[0].OldStart, file.Hunks[0].NewStart = oldStart, newStart
		return file
	}
	tests := []struct {
		name    string
		file    *diff.File
		wantOld string
		wantNew string
	}{
		{"context at line 1", hunk(1, 1, codeLine{diff.Context, "#!/bin/sh"}), "#!/bin/sh", "#!/bin/sh"},
		{"changed first line", hunk(1, 1, codeLine{diff.Delete, "old"}, codeLine{diff.Add, "new"}, codeLine{diff.Context, "rest"}), "old", "new"},
		{"line added above context", hunk(1, 1, codeLine{diff.Add, "new"}, codeLine{diff.Context, "kept"}), "kept", "new"},
		{"added file", hunk(0, 1, codeLine{diff.Add, "new"}), "", "new"},
		{"deleted file", hunk(1, 0, codeLine{diff.Delete, "old"}), "old", ""},
		{"hunk lower down", hunk(40, 40, codeLine{diff.Context, "#!/bin/sh"}), "", ""},
		{"no hunks", &diff.File{}, "", ""},
		{"empty first line above a shebang", hunk(1, 1, codeLine{diff.Context, ""}, codeLine{diff.Context, "#!/bin/sh"}), "", ""},
		{"empty deleted first line above a deleted shebang", hunk(1, 1, codeLine{diff.Delete, ""}, codeLine{diff.Delete, "#!/bin/sh"}, codeLine{diff.Add, "new"}), "", "new"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			oldFirst, newFirst := firstLines(testCase.file)
			if oldFirst != testCase.wantOld || newFirst != testCase.wantNew {
				test.Errorf("firstLines = %q, %q; want %q, %q", oldFirst, newFirst, testCase.wantOld, testCase.wantNew)
			}
		})
	}
}

// TestRowByRowFallbackUsesTheRowsSide: a deleted row of a side too large to
// lex as one block is lexed alone, with the old side's first line, so a
// swapped shebang still colours it in the old language.
func TestRowByRowFallbackUsesTheRowsSide(test *testing.T) {
	firstLineFor := map[string]string{}
	model := New(theme.New(true), WithHighlighter(func(_, firstLine string, lines []string) [][]Span {
		result := make([][]Span, len(lines))
		for i, line := range lines {
			if len(lines) == 1 {
				firstLineFor[line] = firstLine
			}
			result[i] = plain(line)
		}
		return result
	}))
	tooLong := strings.Repeat("x", maximumHighlightLength+1) // the old side can't be one block
	model.SetFile(hunkFile("run", []codeLine{
		{diff.Delete, "#!/bin/sh"},
		{diff.Delete, tooLong},
		{diff.Add, "#!/usr/bin/env python3"},
	}))
	model.SetSize(80, 10)
	model.Render(80, 10)
	if got, ok := firstLineFor["#!/bin/sh"]; !ok || got != "#!/bin/sh" {
		test.Errorf("deleted row lexed with first line %q (lexed alone: %v), want the old side's %q", got, ok, "#!/bin/sh")
	}
}
