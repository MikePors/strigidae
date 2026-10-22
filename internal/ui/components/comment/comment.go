// Package comment renders one comment: a header line (author, age, draft
// marker, read-only reactions) and its Markdown body. The review threads and
// the conversation both use it so a comment looks the same everywhere.
package comment

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

// Comment is what is drawn for one comment.
type Comment struct {
	Author string
	// Action is what the author did ("commented", "approved"); empty for a
	// plain comment, where only the author shows.
	Action    string
	At        time.Time // zero omits the age
	Body      string
	Pending   bool // a draft nobody else can see yet
	Reactions []github.ReactionSummary
	// Muted de-emphasises the comment, for outdated threads.
	Muted bool
}

// Header renders the header line, not truncated.
func Header(th theme.Theme, now time.Time, c Comment) string {
	name := th.Author
	if c.Muted {
		name = th.Muted
	}
	s := name.Render(Author(c.Author))
	if c.Action != "" && !c.Pending {
		s += " " + c.Action
	}
	if !c.At.IsZero() {
		s += th.Muted.Render(" · " + RelTime(now, c.At))
	}
	if c.Pending {
		label := "PENDING (draft, not submitted)"
		if c.Action != "" {
			label = "PENDING " + c.Action + " (draft, not submitted)"
		}
		s += " " + th.Pending.Render(label)
	}
	if r := reactions(c.Reactions); r != "" {
		s += "  " + th.Muted.Render(r)
	}
	return s
}

// Body renders the Markdown body in width columns, or "" for a blank body.
// Muted bodies lose their styling and are drawn in the muted colour.
func Body(th theme.Theme, md *markdown.Renderer, c Comment, width int) string {
	if strings.TrimSpace(c.Body) == "" {
		return ""
	}
	out := md.Render(termtext.CleanBlock(c.Body), width)
	if !c.Muted {
		return out
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = th.Muted.Render(strings.TrimRight(ansi.Strip(l), " "))
	}
	return strings.Join(lines, "\n")
}

// Author is the cleaned display name, "ghost" for a deleted account.
func Author(name string) string {
	if name == "" {
		return "ghost"
	}
	return termtext.Clean(name)
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
