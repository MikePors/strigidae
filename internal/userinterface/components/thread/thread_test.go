package thread

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/markdown"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

var now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func environment() Environment {
	activeTheme := theme.New(true)
	return Environment{Theme: activeTheme, Markdown: markdown.New(activeTheme), Now: now}
}

func TestLines(test *testing.T) {
	thread := github.ReviewThread{
		Path: "a/b.go", Line: 7, IsResolved: true, IsOutdated: true,
		Comments: []github.ReviewComment{
			{
				Author: "", Body: "first\n\nsecond", CreatedAt: now.Add(-time.Hour), Pending: true,
				Reactions: []github.ReactionSummary{{Content: github.ReactionHeart, Count: 3}, {Content: github.ReactionEyes}},
			},
			{Author: "bob", Body: "   ", CreatedAt: now.Add(-time.Minute)},
		},
	}
	tests := []struct {
		name string
		o    Options
		want []string
		not  []string
	}{
		{"collapsed", Options{}, []string{"▸ ghost 2 comments · 1m ago [resolved] [outdated] — first"}, []string{"│"}},
		{"collapsed with location", Options{Location: true}, []string{"▸ a/b.go:7 ghost"}, nil},
		{"expanded", Options{Expanded: true}, []string{"▾ ghost", "│ ghost · 1h ago PENDING", "❤️ 3", "second", "│ bob · 1m ago"}, []string{"👀"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			out := ansi.Strip(strings.Join(Lines(environment(), thread, 80, testCase.o), "\n"))
			for _, word := range testCase.want {
				if !strings.Contains(out, word) {
					test.Errorf("missing %q in:\n%s", word, out)
				}
			}
			for _, name := range testCase.not {
				if strings.Contains(out, name) {
					test.Errorf("unexpected %q in:\n%s", name, out)
				}
			}
		})
	}
}

func TestLinesFitWidth(test *testing.T) {
	thread := github.ReviewThread{
		Path: strings.Repeat("long/", 30) + "f.go", Line: 1,
		Comments: []github.ReviewComment{{Author: strings.Repeat("n", 60), Body: strings.Repeat("word ", 60), CreatedAt: now}},
	}
	for _, options := range []Options{{}, {Expanded: true, Location: true}} {
		for i, line := range Lines(environment(), thread, 40, options) {
			if width := ansi.StringWidth(line); width > 40 {
				test.Errorf("%+v line %d is %d cells wide: %q", options, i, width, ansi.Strip(line))
			}
		}
	}
}

func TestHostileText(test *testing.T) {
	const evil = "x\x1b]0;pwned\x07\x1b[2J\u202e\rend &#x1b;[2J"
	thread := github.ReviewThread{Path: "p" + evil, Line: 1, Comments: []github.ReviewComment{
		{Author: "a" + evil, Body: evil, CreatedAt: now, Reactions: []github.ReactionSummary{{Content: github.ReactionContent("R" + evil), Count: 1}}},
	}}
	for _, options := range []Options{{}, {Expanded: true, Location: true}} {
		out := strings.Join(Lines(environment(), thread, 80, options), "\n")
		plain := regexpSGR(out)
		for _, character := range plain {
			if character == 0x1b || character == 0x07 || character == '\r' || character == 0x202e {
				test.Errorf("%+v: output holds %U: %q", options, character, plain)
			}
		}
	}
}

// regexpSGR removes the SGR styling our own styles emit.
func regexpSGR(text string) string {
	var builder strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == 0x1b && i+1 < len(text) && text[i+1] == '[' {
			j := i + 2
			for j < len(text) && (text[j] == ';' || text[j] == ':' || (text[j] >= '0' && text[j] <= '9')) {
				j++
			}
			if j < len(text) && text[j] == 'm' {
				i = j
				continue
			}
		}
		builder.WriteByte(text[i])
	}
	return builder.String()
}
