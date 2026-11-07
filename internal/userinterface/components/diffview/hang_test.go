package diffview

import (
	"fmt"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
)

// hostileLines are one-liners that leave a construct open: a comment, a
// string, a heredoc, an escape.
var hostileLines = []string{"/* a", `"a`, "<<EOF", `\`}

// sweepTimeout is how long all lexers together may take on hostileLines
// before the ones still running count as hanging. Every finite lexer takes
// well under 100 ms (compiling its rules included); the margin is for the
// race detector and busy machines.
const sweepTimeout = 10 * time.Second

// TestNoRegisteredLexerIsBroken lexes the hostile one-liners, alone and as a
// block of two, with every lexer Chroma registers except those broken lists,
// and fails if one panics or does not finish. A lexer that hangs is left
// spinning until the test binary exits: Go cannot stop it.
func TestNoRegisteredLexerIsBroken(test *testing.T) {
	type run struct {
		name     string
		finished chan any // receives the recovered panic, or nil
	}
	var runs []run
	for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
		if broken(lexer) {
			continue
		}
		current := run{name: lexer.Config().Name, finished: make(chan any, 1)}
		runs = append(runs, current)
		go func() {
			defer func() { current.finished <- recover() }()
			coalesced := chroma.Coalesce(lexer)
			for _, line := range hostileLines {
				lexLine(coalesced, line)
				lexBlock(coalesced, []string{line, line}, time.Now)
			}
		}()
	}
	timeout := time.After(sweepTimeout)
	for _, current := range runs {
		select {
		case recovered := <-current.finished:
			if recovered != nil {
				test.Errorf("lexer %q panicked on the hostile lines: %v; add it to broken and report it to Chroma", current.name, recovered)
			}
		case <-timeout:
			test.Errorf("lexer %q did not finish the hostile lines within %v; add it to broken and report it to Chroma", current.name, sweepTimeout)
		}
	}
}

// TestBrokenLexersAreRegistered keeps the broken list honest: each name is
// still a registered lexer, so a rename in Chroma cannot silently let it run.
func TestBrokenLexersAreRegistered(test *testing.T) {
	for _, name := range []string{"JSONata", "Jungle", "YANG"} {
		lexer := lexers.Get(name)
		if lexer == nil || lexer.Config().Name != name || !broken(lexer) {
			test.Errorf("lexer %q is not registered under that name or not listed in broken", name)
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
			if diff := cmp.Diff(want, got); diff != "" {
				test.Errorf("spans (-want +got):\n%s", diff)
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

// TestLexBlockStopsAtTheBudget: with a clock that moves a tenth of the budget
// per read, block lexing stops once the budget has passed. The lines before the
// stop keep the block's spans; every line from there on is lexed on its own,
// with its text intact. The lines are pairs whose comment spans both, so the
// two ways of lexing differ: in the block the second line opens with a
// comment, on its own it has none.
func TestLexBlockStopsAtTheBudget(test *testing.T) {
	lexer := chroma.Coalesce(lexers.Get("go"))
	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprintf("value%d := %d /* note", i, i), fmt.Sprintf("end */ next%d := %d", i, i))
	}
	now, reads := steppingClock(blockBudget / 10)
	got := lexBlock(lexer, lines, now)

	// One read starts the budget, then one per tokensPerClockRead tokens
	// until it has passed.
	if limit := 12; reads() > limit {
		test.Errorf("clock read %d times, want lexing to stop by %d", reads(), limit)
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

// TestLexBlockWithinBudgetIsUnchanged: a block that finishes in time is
// lexed as one piece, so a comment spanning lines is a comment on each.
func TestLexBlockWithinBudgetIsUnchanged(test *testing.T) {
	lines := []string{"/* start", "middle", "end */"}
	now, _ := steppingClock(time.Microsecond)
	for i, spans := range lexBlock(chroma.Coalesce(lexers.Get("go")), lines, now) {
		if !allIn(spans, chroma.Comment) {
			test.Errorf("line %d = %+v, want a comment", i, spans)
		}
	}
}

// BenchmarkHighlightHostileBlock lexes a block-sized side of lines that each
// open a block comment, with the real clock: the budget bounds it.
func BenchmarkHighlightHostileBlock(benchmark *testing.B) {
	lexer := chroma.Coalesce(lexers.Match("x.go"))
	lines := unclosedCommentLines(maximumBlockSize / len("value0000 := 0000 /* note\n"))
	if !fitsBlock(lines) {
		benchmark.Fatal("the hostile block does not fit one block")
	}
	for range benchmark.N {
		lexBlock(lexer, lines, time.Now)
	}
}
