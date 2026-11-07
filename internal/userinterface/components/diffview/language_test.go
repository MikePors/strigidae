package diffview

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
)

// lexedAs returns lines as the named Chroma lexer lexes them as one block.
func lexedAs(test *testing.T, name string, lines []string) [][]Span {
	test.Helper()
	lexer := lexers.Get(name)
	if lexer == nil {
		test.Fatalf("no lexer called %q", name)
	}
	return lexBlock(chroma.Coalesce(lexer), lines)
}

// allPlain returns lines as a Highlighter reports a file with no language.
func allPlain(lines []string) [][]Span {
	spans := make([][]Span, len(lines))
	for i, line := range lines {
		spans[i] = plain(line)
	}
	return spans
}

func TestLanguageFromContent(test *testing.T) {
	pythonLines := []string{"#!/usr/bin/env python3", "import os", "", "def main():", "    return os.getcwd()  # here"}
	shellLines := []string{"#!/bin/sh", "set -e", `if [ -n "$HOME" ]; then echo "$HOME"; fi`}
	tests := []struct {
		name       string
		path       string
		lines      []string
		startsFile bool
		language   string // the Chroma lexer expected; "" for plain
	}{
		{name: "python through env", path: "bin/release", lines: pythonLines, startsFile: true, language: "python"},
		{name: "shell", path: "bin/release", lines: shellLines, startsFile: true, language: "bash"},
		{name: "interpreter with a version", path: "run", lines: []string{"#!/usr/bin/python3.11", "print(1)"}, startsFile: true, language: "python"},
		{name: "env with a flag and a variable", path: "run", lines: []string{"#!/usr/bin/env -S LANG=C ruby -w", "puts 1"}, startsFile: true, language: "ruby"},
		{name: "spaces after the marker", path: "run", lines: []string{"#! /usr/bin/perl", "print 1;"}, startsFile: true, language: "perl"},
		{name: "no shebang but content analysis knows it", path: "main", lines: []string{"package main"}, startsFile: true, language: "go"},
		{name: "a block that does not start the file stays plain", path: "bin/release", lines: pythonLines, startsFile: false},
		{name: "an unknown interpreter stays plain", path: "bin/release", lines: []string{"#!/opt/odd/frobnicate", "x"}, startsFile: true},
		{name: "a bare marker stays plain", path: "bin/release", lines: []string{"#!", "x"}, startsFile: true},
		{name: "env alone stays plain", path: "bin/release", lines: []string{"#!/usr/bin/env", "x"}, startsFile: true},
		{name: "plain text stays plain", path: "NOTES", lines: []string{"just words", "and more"}, startsFile: true},
		{name: "a name match wins over a misleading shebang", path: "tool.py", lines: pythonLines[1:], startsFile: true, language: "python"},
		{name: "a lexer that can hang is never chosen from content", path: "bin/release", lines: []string{"#!/usr/bin/env jsonata", "$x"}, startsFile: true},
		{name: "the other lexer that can hang", path: "bin/release", lines: []string{"#!/usr/bin/env jungle", "x"}, startsFile: true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			want := allPlain(testCase.lines)
			if testCase.language != "" {
				want = lexedAs(test, testCase.language, testCase.lines)
			}
			got := ChromaHighlighter()(testCase.path, testCase.lines, testCase.startsFile)
			if diff := cmp.Diff(want, got); diff != "" {
				test.Errorf("spans differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNameMatchWinsOverShebang proves a file name that picks a lexer is not
// overridden by the first line: shell text in a .py file is lexed as Python.
func TestNameMatchWinsOverShebang(test *testing.T) {
	lines := []string{"#!/bin/sh", "def f(): pass"}
	got := ChromaHighlighter()("tool.py", lines, true)
	if diff := cmp.Diff(lexedAs(test, "python", lines), got); diff != "" {
		test.Errorf("spans differ (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(lexedAs(test, "bash", lines), got); diff == "" {
		test.Error("the shebang was used although the name matched")
	}
}

// TestContentLanguageIsRemembered proves the language found at the top of a
// file colours the later hunks of the same path, and that a guess that found
// nothing is not remembered (the new side may gain the shebang).
func TestContentLanguageIsRemembered(test *testing.T) {
	highlighter := ChromaHighlighter()
	later := []string{"/* a", "b */"}
	if diff := cmp.Diff(allPlain(later), highlighter("bin/run", later, false)); diff != "" {
		test.Errorf("before the top of the file is seen (-want +got):\n%s", diff)
	}
	if got := highlighter("bin/run", []string{"echo old"}, true); !allPlainSpans(got) {
		test.Errorf("a side without a shebang was coloured: %v", got)
	}
	top := []string{"#!/usr/bin/env python3", "import os"}
	if diff := cmp.Diff(lexedAs(test, "python", top), highlighter("bin/run", top, true)); diff != "" {
		test.Errorf("a side that gained a shebang (-want +got):\n%s", diff)
	}
	lines := []string{`"""doc`, `more"""`}
	if diff := cmp.Diff(lexedAs(test, "python", lines), highlighter("bin/run", lines, false)); diff != "" {
		test.Errorf("a later hunk of the same path (-want +got):\n%s", diff)
	}
	if got := highlighter("bin/other", lines, false); !allPlainSpans(got) {
		test.Errorf("another path was coloured: %v", got)
	}
}

func allPlainSpans(spans [][]Span) bool {
	for _, line := range spans {
		for _, span := range line {
			if span.Token != chroma.Text {
				return false
			}
		}
	}
	return true
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
