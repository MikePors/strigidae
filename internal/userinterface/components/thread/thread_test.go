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
	th := theme.New(true)
	return Environment{Theme: th, Markdown: markdown.New(th), Now: now}
}

func TestLines(t *testing.T) {
	th := github.ReviewThread{
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ansi.Strip(strings.Join(Lines(environment(), th, 80, tt.o), "\n"))
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in:\n%s", w, out)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(out, n) {
					t.Errorf("unexpected %q in:\n%s", n, out)
				}
			}
		})
	}
}

func TestLinesFitWidth(t *testing.T) {
	th := github.ReviewThread{
		Path: strings.Repeat("long/", 30) + "f.go", Line: 1,
		Comments: []github.ReviewComment{{Author: strings.Repeat("n", 60), Body: strings.Repeat("word ", 60), CreatedAt: now}},
	}
	for _, o := range []Options{{}, {Expanded: true, Location: true}} {
		for i, l := range Lines(environment(), th, 40, o) {
			if w := ansi.StringWidth(l); w > 40 {
				t.Errorf("%+v line %d is %d cells wide: %q", o, i, w, ansi.Strip(l))
			}
		}
	}
}

func TestHostileText(t *testing.T) {
	const evil = "x\x1b]0;pwned\x07\x1b[2J\u202e\rend &#x1b;[2J"
	th := github.ReviewThread{Path: "p" + evil, Line: 1, Comments: []github.ReviewComment{
		{Author: "a" + evil, Body: evil, CreatedAt: now, Reactions: []github.ReactionSummary{{Content: github.ReactionContent("R" + evil), Count: 1}}},
	}}
	for _, o := range []Options{{}, {Expanded: true, Location: true}} {
		out := strings.Join(Lines(environment(), th, 80, o), "\n")
		plain := regexpSGR(out)
		for _, r := range plain {
			if r == 0x1b || r == 0x07 || r == '\r' || r == 0x202e {
				t.Errorf("%+v: output holds %U: %q", o, r, plain)
			}
		}
	}
}

// regexpSGR removes the SGR styling our own styles emit.
func regexpSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] == ';' || s[j] == ':' || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
