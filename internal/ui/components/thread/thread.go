// Package thread renders a review thread as lines of styled text: a header
// line that is always shown, and, when expanded, every comment with its author,
// age, read-only reactions and Markdown body. Hosts (the diff pane, the
// conversation) place the lines; the package knows nothing about scrolling.
package thread

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/components/markdown"
	"github.com/MikePors/strigidae/internal/ui/termtext"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// Env is what rendering needs besides the thread.
type Env struct {
	Theme    theme.Theme
	Markdown *markdown.Renderer
	Now      time.Time
}

// Options say how one thread is drawn.
type Options struct {
	// Expanded shows every comment below the header line.
	Expanded bool
	// Location puts "path:line" in the header, for lists where the thread is
	// not already next to its code.
	Location bool
}

// DefaultExpanded is whether a thread starts expanded: resolved ones do not.
func DefaultExpanded(t github.ReviewThread) bool { return !t.IsResolved }

// Quiet reports whether a thread is de-emphasised: resolved or outdated.
func Quiet(t github.ReviewThread) bool { return t.IsResolved || t.IsOutdated }

// Lines renders t in at most width columns per line. The first line is the
// header; its first cell is the fold glyph. Lines are not padded.
func Lines(e Env, t github.ReviewThread, width int, o Options) []string {
	width = max(width, 8)
	out := []string{header(e, t, width, o)}
	if !o.Expanded {
		return out
	}
	th := e.Theme
	bar := th.ThreadBar.Render("│ ")
	for i, c := range t.Comments {
		out = append(out, ansi.Truncate(bar+commentHeader(e, c, t.IsOutdated), width, "…"))
		if strings.TrimSpace(c.Body) == "" {
			continue
		}
		for _, l := range strings.Split(e.Markdown.Render(termtext.CleanBlock(c.Body), width-2), "\n") {
			if t.IsOutdated {
				l = th.Muted.Render(strings.TrimRight(ansi.Strip(l), " "))
			}
			out = append(out, bar+l)
		}
		if i < len(t.Comments)-1 {
			out = append(out, strings.TrimRight(bar, " "))
		}
	}
	return out
}

func header(e Env, t github.ReviewThread, width int, o Options) string {
	th := e.Theme
	glyph := "▸"
	if o.Expanded {
		glyph = "▾"
	}
	style := th.ThreadMarker
	if Quiet(t) {
		style = th.Muted
	}
	parts := []string{style.Render(glyph)}
	if o.Location {
		parts = append(parts, th.Normal.Render(location(t)))
	}
	n := len(t.Comments)
	if n > 0 {
		parts = append(parts, th.Author.Render(author(t.Comments[0].Author)))
	}
	meta := fmt.Sprintf("%d comment", n)
	if n != 1 {
		meta += "s"
	}
	if n > 0 {
		meta += " · " + RelTime(e.Now, t.Comments[n-1].CreatedAt)
	}
	parts = append(parts, th.Muted.Render(meta))
	if t.IsResolved {
		parts = append(parts, th.Muted.Render("[resolved]"))
	}
	if t.IsOutdated {
		parts = append(parts, th.Muted.Render("[outdated]"))
	}
	if t.SubjectType == github.SubjectFile {
		parts = append(parts, th.Muted.Render("[file]"))
	}
	line := strings.Join(parts, " ")
	if !o.Expanded && n > 0 {
		if p := preview(t.Comments[0].Body); p != "" {
			line += th.Muted.Render(" — " + p)
		}
	}
	return ansi.Truncate(line, width, "…")
}

func commentHeader(e Env, c github.ReviewComment, outdated bool) string {
	th := e.Theme
	name := th.Author
	if outdated {
		name = th.Muted
	}
	s := name.Render(author(c.Author)) + th.Muted.Render(" · "+RelTime(e.Now, c.CreatedAt))
	if c.Pending {
		s += " " + th.Pending.Render("PENDING (draft, not submitted)")
	}
	if r := reactions(c.Reactions); r != "" {
		s += "  " + th.Muted.Render(r)
	}
	return s
}

// reactions formats the non-zero reaction counts: "👍 2 👀 1".
func reactions(rs []github.ReactionSummary) string {
	var parts []string
	for _, r := range rs {
		if r.Count > 0 {
			parts = append(parts, termtext.Clean(r.Content.Emoji())+" "+strconv.Itoa(r.Count))
		}
	}
	return strings.Join(parts, " ")
}

func location(t github.ReviewThread) string {
	loc := termtext.Clean(t.Path)
	line := t.Line
	if line == 0 {
		line = t.OriginalLine
	}
	if line > 0 {
		loc += ":" + strconv.Itoa(line)
	}
	return loc
}

func author(name string) string {
	if name == "" {
		return "ghost"
	}
	return termtext.Clean(name)
}

// preview is the first non-empty line of a body, cleaned, for collapsed threads.
func preview(body string) string {
	for l := range strings.SplitSeq(termtext.CleanBlock(body), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// RelTime formats t relative to now ("5m ago").
func RelTime(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy ago", int(d/(365*24*time.Hour)))
	}
}
