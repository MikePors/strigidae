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
	"github.com/MikePors/strigidae/internal/userinterface/components/markdown"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
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
func Header(activeTheme theme.Theme, now time.Time, comment Comment) string {
	name := activeTheme.Author
	if comment.Muted {
		name = activeTheme.Muted
	}
	text := name.Render(Author(comment.Author))
	if comment.Action != "" && !comment.Pending {
		text += " " + comment.Action
	}
	if !comment.At.IsZero() {
		text += activeTheme.Muted.Render(" · " + RelativeTime(now, comment.At))
	}
	if comment.Pending {
		label := "PENDING (draft, not submitted)"
		if comment.Action != "" {
			label = "PENDING " + comment.Action + " (draft, not submitted)"
		}
		text += " " + activeTheme.Pending.Render(label)
	}
	if reactionText := ReactionText(activeTheme, comment.Reactions); reactionText != "" {
		text += "  " + reactionText
	}
	return text
}

// ReactionText renders the non-zero reaction counts in the muted style, or ""
// when there are none.
func ReactionText(activeTheme theme.Theme, reactionList []github.ReactionSummary) string {
	if text := reactions(reactionList); text != "" {
		return activeTheme.Muted.Render(text)
	}
	return ""
}

// Body renders the Markdown body in width columns, or "" for a blank body.
// Muted bodies lose their styling and are drawn in the muted colour.
func Body(activeTheme theme.Theme, markdownRenderer *markdown.Renderer, comment Comment, width int) string {
	if strings.TrimSpace(comment.Body) == "" {
		return ""
	}
	out := markdownRenderer.Render(terminaltext.CleanBlock(comment.Body), width)
	if !comment.Muted {
		return out
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = activeTheme.Muted.Render(strings.TrimRight(ansi.Strip(line), " "))
	}
	return strings.Join(lines, "\n")
}

// Author is the cleaned display name, "ghost" for a deleted account.
func Author(name string) string {
	if name == "" {
		return "ghost"
	}
	return terminaltext.Clean(name)
}

// reactions formats the non-zero reaction counts: "👍 2 👀 1".
func reactions(reactionList []github.ReactionSummary) string {
	var parts []string
	for _, reaction := range reactionList {
		if reaction.Count > 0 {
			parts = append(parts, terminaltext.Clean(reaction.Content.Emoji())+" "+strconv.Itoa(reaction.Count))
		}
	}
	return strings.Join(parts, " ")
}

// RelativeTime formats t relative to now ("5m ago").
func RelativeTime(now, timestamp time.Time) string {
	duration := now.Sub(timestamp)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm ago", int(duration/time.Minute))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(duration/time.Hour))
	case duration < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(duration/(24*time.Hour)))
	case duration < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(duration/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy ago", int(duration/(365*24*time.Hour)))
	}
}
