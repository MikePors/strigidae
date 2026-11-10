package diffview

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/lexerguard"
)

// hostileLines are one-liners that leave a construct open: a comment, a
// string, a heredoc, an escape.
var hostileLines = []string{"/* a", `"a`, "<<EOF", `\`, "/* a /* b */", "/* a /* b"}

// sweepTimeout is how long all lexers together may take on hostileLines
// before the ones still running count as hanging. Every finite lexer takes
// well under 100 ms (compiling its rules included); the margin is for the
// race detector and busy machines.
const sweepTimeout = 10 * time.Second

// TestNoRegisteredLexerIsBroken lexes the hostile one-liners, alone and as a
// block of two, with every lexer Chroma registers (the broken ones are plain
// text by then, see lexerguard), and fails if one panics or does not finish.
// A lexer that hangs is left spinning until the test binary exits: Go cannot
// stop it.
func TestNoRegisteredLexerIsBroken(test *testing.T) {
	type run struct {
		name     string
		finished chan any // receives the recovered panic, or nil
	}
	var runs []run
	for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
		current := run{name: lexer.Config().Name, finished: make(chan any, 1)}
		runs = append(runs, current)
		go func() {
			defer func() { current.finished <- recover() }()
			for _, line := range hostileLines {
				lexLine(lexer, line)
				lexBlock(lexer, []string{line, line}, time.Now)
			}
		}()
	}
	// A context's Done stays closed, so every lexer still running at the
	// deadline is named, not just the first.
	ctx, cancel := context.WithTimeout(context.Background(), sweepTimeout)
	defer cancel()
	for _, current := range runs {
		select {
		case recovered := <-current.finished:
			reportPanic(test, current.name, recovered)
		case <-ctx.Done():
			// Done and finished may both be ready; select picks either.
			select {
			case recovered := <-current.finished:
				reportPanic(test, current.name, recovered)
			default:
				test.Errorf("lexer %q did not finish the hostile lines within %v; add it to lexerguard and report it to Chroma", current.name, sweepTimeout)
			}
		}
	}
}

// reportPanic fails the test if the lexer called name panicked with recovered.
func reportPanic(test *testing.T, name string, recovered any) {
	test.Helper()
	if recovered != nil {
		test.Errorf("lexer %q panicked on the hostile lines: %v; add it to lexerguard and report it to Chroma", name, recovered)
	}
}

// TestBrokenLexersAreCountedBroken: each name lexerguard lists resolves to a
// lexer that diffview counts as broken, so its files stay plain. That the
// lexer is the plain replacement is lexerguard's test; the sweep above lexes
// the hostile lines with it.
func TestBrokenLexersAreCountedBroken(test *testing.T) {
	for _, name := range lexerguard.Names() {
		lexer := lexers.Get(name)
		if lexer == nil || lexer.Config().Name != name || !broken(lexer) {
			test.Errorf("lexer %q is not registered under that name or not counted as broken", name)
		}
	}
}

// TestMarkdownFenceWithBrokenLexerRenders: a Markdown file whose fence names
// a lexer that used to hang or panic is drawn, with its text intact.
func TestMarkdownFenceWithBrokenLexerRenders(test *testing.T) {
	for _, tag := range []string{"yang", "YANG", "jsonata", "jungle"} {
		for _, text := range []string{"/* a /* b", "/* a /* b */", "/* a"} {
			test.Run(tag+" "+text, func(test *testing.T) {
				lines := []codeLine{{diff.Add, "```" + tag}, {diff.Add, text}, {diff.Add, "```"}}
				got := rowSpans(test, hunkFile("README.md", lines))
				if len(got) != len(lines) {
					test.Fatalf("%d rows, want %d", len(got), len(lines))
				}
				if joined(got[1]) != text {
					test.Errorf("fence line reads %q, want %q", joined(got[1]), text)
				}
			})
		}
	}
}

// TestBrokenLexerFilesArePlain: a file whose name selects a broken lexer is
// drawn as plain text, and drawing it returns.
func TestBrokenLexerFilesArePlain(test *testing.T) {
	lines := []codeLine{{diff.Add, "/* a"}, {diff.Add, "/* a"}}
	for _, path := range []string{"query.jsonata", "monkey.jungle", "model.yang"} {
		test.Run(path, func(test *testing.T) {
			got := rowSpans(test, hunkFile(path, lines))
			want := [][]Span{plain("/* a"), plain("/* a")}
			if difference := cmp.Diff(want, got); difference != "" {
				test.Errorf("spans (-want +got):\n%s", difference)
			}
		})
	}
}

// steppingClock returns a clock that moves on by step every time it is read,
// and a function reporting how often it was read.
func steppingClock(step time.Duration) (now func() time.Time, reads func() int) {
	current := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	count := 0
	now = func() time.Time {
		count++
		current = current.Add(step)
		return current
	}
	return now, func() int { return count }
}

// unclosedCommentLines returns count Go lines that each open a block comment
// and never close it: lexed as one block, each "/*" scans to the end of it.
func unclosedCommentLines(count int) []string {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("value%d := %d /* note", i, i)
	}
	return lines
}

// budgetReads is how often lexBlock reads a steppingClock that moves a tenth
// of the budget per read: once to start the budget, then every
// tokensPerClockRead tokens until the eleventh tenth has passed.
const budgetReads = 12

// TestLexBlockStopsAtTheBudget: with a clock that moves a tenth of the budget
// per read, block lexing stops once the budget has passed. The lines before
// the stop keep the block's spans; every line from there on is lexed on its
// own, with its text intact. The lines are pairs whose comment spans both,
// so the two ways of lexing differ: in the block the second line opens with a
// comment, on its own it has none.
func TestLexBlockStopsAtTheBudget(test *testing.T) {
	lexer := lexers.Get("go")
	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprintf("value%d := %d /* note", i, i), fmt.Sprintf("end */ next%d := %d", i, i))
	}
	now, reads := steppingClock(blockBudget / 10)
	got := lexBlock(lexer, lines, now)

	if reads() != budgetReads {
		test.Errorf("clock read %d times, want %d: lexing should stop when the budget has passed", reads(), budgetReads)
	}
	unbounded := lexBlock(lexer, lines, func() time.Time { return time.Time{} })
	stopped := 0
	for stopped < len(lines) && cmp.Equal(got[stopped], unbounded[stopped]) {
		stopped++
	}
	if stopped == 0 || stopped == len(lines) {
		test.Fatalf("lexing stopped at line %d of %d, want somewhere inside the block", stopped, len(lines))
	}
	for i := stopped; i < len(lines); i++ {
		if want := lexLine(lexer, lines[i]); !cmp.Equal(got[i], want) {
			test.Errorf("line %d after the budget = %+v, want it lexed on its own: %+v", i, got[i], want)
		}
	}
	for i, line := range lines {
		if joined(got[i]) != line {
			test.Errorf("line %d reads %q, want %q", i, joined(got[i]), line)
		}
		if !anyIn(got[i], chroma.Keyword) && !anyIn(got[i], chroma.Name) && !anyIn(got[i], chroma.Comment) {
			test.Errorf("line %d is not highlighted: %+v", i, got[i])
		}
	}
}

// TestLexBlockBudgetCountsTokensBeforeCoalescing: lines of "/*+" are all
// operators, which Coalesce merges into tokens of up to 8 KiB, each holding
// thousands of slow scans. The budget still stops lexing, because it counts
// the tokens before they are merged, and every line keeps its text.
func TestLexBlockBudgetCountsTokensBeforeCoalescing(test *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("/*+", 30)
	}
	now, reads := steppingClock(blockBudget / 10)
	got := lexBlock(lexers.Get("go"), lines, now)
	if reads() != budgetReads {
		test.Errorf("clock read %d times, want %d: lexing should stop when the budget has passed", reads(), budgetReads)
	}
	for i, line := range lines {
		if joined(got[i]) != line {
			test.Errorf("line %d reads %q, want %q", i, joined(got[i]), line)
		}
	}
}

// TestLexBlockWithinBudgetIsUnchanged: a block that finishes in time is
// lexed as one piece, so a comment spanning lines is a comment on each.
func TestLexBlockWithinBudgetIsUnchanged(test *testing.T) {
	lines := []string{"/* start", "middle", "end */"}
	now, _ := steppingClock(time.Microsecond)
	for i, spans := range lexBlock(lexers.Get("go"), lines, now) {
		if !allIn(spans, chroma.Comment) {
			test.Errorf("line %d = %+v, want a comment", i, spans)
		}
	}
}

// BenchmarkHighlightHostileBlock lexes a block-sized side of lines that each
// open a block comment, with the real clock: the budget bounds it.
func BenchmarkHighlightHostileBlock(benchmark *testing.B) {
	lexer := lexers.Match("x.go")
	lines := unclosedCommentLines(maximumBlockSize / len("value0000 := 0000 /* note\n"))
	if !fitsBlock(lines) {
		benchmark.Fatal("the hostile block does not fit one block")
	}
	for range benchmark.N {
		lexBlock(lexer, lines, time.Now)
	}
}
