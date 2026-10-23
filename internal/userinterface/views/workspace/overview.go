package workspace

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/comment"
	"github.com/MikePors/strigidae/internal/userinterface/components/markdown"
	"github.com/MikePors/strigidae/internal/userinterface/components/thread"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// overview is the right pane's content while Overview is selected: the
// description followed by the conversation, in a scrollable viewport.
type overview struct {
	viewport viewport.Model
	width    int       // content width the viewport was built for
	builtAt  time.Time // minute the content was built in; relative times age with it
	dirty    bool
	sections []int // first line of the description and of the conversation
}

func newOverview() overview {
	return overview{viewport: viewport.New(), dirty: true}
}

// invalidate forces a rebuild before the next render.
func (overview *overview) invalidate() { overview.dirty = true }

// ensure sizes the viewport and rebuilds the content when the data, theme or
// width changed. Markdown rendering is cached by the renderer.
func (overview *overview) ensure(theme theme.Theme, markdownSource *markdown.Renderer, detail *github.PullRequestDetail, threads threadsView, now time.Time, width, height int) {
	overview.viewport.SetWidth(width)
	overview.viewport.SetHeight(height)
	minute := now.Truncate(time.Minute)
	if detail == nil || (!overview.dirty && overview.width == width && minute.Equal(overview.builtAt)) {
		return
	}
	var builder strings.Builder
	overview.sections = overview.sections[:0]
	line := 0
	add := func(text string) {
		builder.WriteString(text)
		builder.WriteString("\n")
		line += strings.Count(text, "\n") + 1
	}

	overview.sections = append(overview.sections, line)
	add(theme.SectionHeading.Render("Description"))
	if strings.TrimSpace(detail.Body) == "" {
		add(theme.Muted.Render("No description provided."))
	} else {
		add(markdownSource.Render(terminaltext.CleanBlock(detail.Body), width))
	}
	add("")

	overview.sections = append(overview.sections, line)
	items := conversation(detail)
	add(theme.SectionHeading.Render(fmt.Sprintf("Conversation (%d)", len(items))))
	if len(items) == 0 {
		add(theme.Muted.Render("No comments yet."))
	}
	for _, item := range items {
		add(comment.Header(theme, now, item.comment))
		if body := comment.Body(theme, markdownSource, item.comment, width); body != "" {
			add(body)
		}
		add("")
	}

	overview.sections = append(overview.sections, line)
	switch {
	case !threads.loaded && threads.pending:
		add(theme.SectionHeading.Render("Review threads"))
		add(theme.Muted.Render("Loading review threads…"))
	case !threads.loaded && threads.err != nil:
		add(theme.SectionHeading.Render("Review threads"))
		add(theme.ErrorText.Render(ansi.Wrap("Could not load review threads: "+terminaltext.SafeError(threads.err).Error(), width, "")))
	default:
		add(theme.SectionHeading.Render(fmt.Sprintf("Review threads (%d)", len(threads.list))))
		if len(threads.list) == 0 {
			add(theme.Muted.Render("No review threads."))
		}
	}
	environment := thread.Environment{Theme: theme, Markdown: markdownSource, Now: now}
	for _, reviewThread := range threads.list {
		add(strings.Join(thread.Lines(environment, reviewThread, width, thread.Options{Expanded: true, Location: true}), "\n"))
		add("")
	}

	overview.viewport.SetContent(strings.TrimRight(builder.String(), "\n"))
	overview.width, overview.builtAt, overview.dirty = width, minute, false
}

// jump scrolls to the next (delta > 0) or previous section start.
func (overview *overview) jump(delta int) {
	row := overview.viewport.YOffset()
	if delta > 0 {
		for _, size := range overview.sections {
			if size > row {
				overview.viewport.SetYOffset(size)
				return
			}
		}
		return
	}
	for _, size := range slices.Backward(overview.sections) {
		if size < row {
			overview.viewport.SetYOffset(size)
			return
		}
	}
}

// threadsView is the review threads and how loading them stands, as the
// Overview needs them.
type threadsView struct {
	list    []github.ReviewThread
	loaded  bool // a load has succeeded
	pending bool // a load is in flight
	err     error
}

// entry is one item of the conversation: a top-level comment or a review.
type entry struct {
	comment comment.Comment
	at      time.Time
}

// conversation merges comments and review summaries chronologically. Pending
// reviews have no submission time and come last. Reviews without a body that
// only wrap inline comments ("commented") are left out; verdicts are kept.
func conversation(detail *github.PullRequestDetail) []entry {
	var out []entry
	for _, reviewComment := range detail.Comments {
		out = append(out, entry{at: reviewComment.CreatedAt, comment: comment.Comment{Author: reviewComment.Author, Action: "commented", At: reviewComment.CreatedAt, Body: reviewComment.Body, Reactions: reviewComment.Reactions}})
	}
	for _, review := range detail.Reviews {
		listEntry := entry{at: review.SubmittedAt, comment: comment.Comment{Author: review.Author, Action: reviewAction(review.State), At: review.SubmittedAt, Body: review.Body}}
		switch {
		case review.State == github.ReviewPending || review.SubmittedAt.IsZero():
			listEntry.comment.Pending, listEntry.comment.Action, listEntry.comment.At = true, "review", time.Time{}
		case review.State == github.ReviewCommented && strings.TrimSpace(review.Body) == "":
			continue
		}
		out = append(out, listEntry)
	}
	slices.SortStableFunc(out, func(first, entry entry) int {
		if first.comment.Pending != entry.comment.Pending {
			if first.comment.Pending {
				return 1
			}
			return -1
		}
		return first.at.Compare(entry.at)
	})
	return out
}

func reviewAction(state github.ReviewState) string {
	switch state {
	case github.ReviewApproved:
		return "approved"
	case github.ReviewChangesRequested:
		return "requested changes"
	case github.ReviewDismissed:
		return "review dismissed"
	default:
		return "reviewed"
	}
}

// sortThreads orders threads by file, then line, keeping the server's order
// for threads on the same line. It returns a copy.
func sortThreads(threads []github.ReviewThread) []github.ReviewThread {
	out := slices.Clone(threads)
	line := func(thread github.ReviewThread) int {
		if thread.Line > 0 {
			return thread.Line
		}
		return thread.OriginalLine
	}
	slices.SortStableFunc(out, func(first, thread github.ReviewThread) int {
		if count := strings.Compare(first.Path, thread.Path); count != 0 {
			return count
		}
		return line(first) - line(thread)
	})
	return out
}
