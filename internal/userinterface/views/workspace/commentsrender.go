package workspace

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/components/comment"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/components/thread"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

func (view *View) renderComments(width, height int) []string {
	view.syncComments()
	pane := &view.comments
	pane.viewport.SetWidth(max(1, width))

	status := fmt.Sprintf("%d conversations · %d unresolved · %d draft comments", pane.total, pane.open, pane.drafts)
	if pane.unresolved {
		status = fmt.Sprintf("Unresolved · %d of %d conversations", len(pane.items), pane.total)
	}
	if !view.threadLoaded || view.threadPending || view.pullRequestPending || view.threadErr != nil || view.detailErr != nil {
		status = "Partial · " + status
	}
	var notices []string
	if view.pullRequestPending {
		notices = append(notices, "Loading PR conversation…")
	}
	if view.detailErr != nil {
		notices = append(notices, "PR conversation could not be refreshed. R retry.")
	}
	if view.threadPending {
		notices = append(notices, "Loading review threads…")
	}
	if view.threadErr != nil {
		notices = append(notices, "Review threads could not be loaded. R retry.")
	}
	header := []string{view.theme.Muted.Render(ansi.Truncate(status, max(1, width), "…"))}
	for _, notice := range notices {
		header = append(header, view.theme.Muted.Render(ansi.Truncate(notice, max(1, width), "…")))
	}
	pane.viewport.SetHeight(max(0, height-len(header)))
	var lines []string
	add := func(text string) { lines = append(lines, strings.Split(text, "\n")...) }
	pane.starts = nil
	pane.matchingCommentStart = -1
	group := -1
	for i, item := range pane.items {
		if group != item.group {
			group = item.group
			add(view.theme.SectionHeading.Render([]string{"Drafts (not submitted)", "Published", "Local (not on GitHub)"}[group]))
		}
		pane.starts = append(pane.starts, len(lines))
		selected := i == pane.window.Cursor()
		lead := "  "
		if pane.search.Marked(i) && view.options.HighlightSearch() {
			lead = "• "
		}
		if selected {
			lead = "> "
		}
		var body []string
		var owners []int
		matchingComment := -1
		if item.thread != nil {
			if selected && pane.search.Active() {
				texts := make([]string, len(item.thread.Comments))
				for i, currentComment := range item.thread.Comments {
					texts[i] = terminaltext.CleanBlock(threadSearchText(*item.thread) + " " + currentComment.Author + " " + currentComment.Body)
					if currentComment.Pending {
						texts[i] += " draft"
					}
				}
				if matches := search.Match(pane.search.Query(), texts); len(matches) > 0 {
					matchingComment = matches[0]
				}
			}
			body, owners = thread.Layout(thread.Environment{Theme: view.theme, Markdown: view.markdownRenderer, Now: view.now()}, *item.thread, max(8, width-2), thread.Options{Expanded: selected, Location: true})
			if selected && !item.thread.IsResolved && !item.thread.Local.IsLocal() {
				body = append(body[:1], append([]string{view.theme.Muted.Render("[unresolved]")}, body[1:]...)...)
				owners = append(owners[:1], append([]int{-1}, owners[1:]...)...)
			}
			if selected && item.thread.IsOutdated && len(item.thread.Comments) > 0 && item.thread.Comments[0].DiffHunk != "" {
				body = append(body, view.theme.Muted.Render("Original context:"))
				for _, line := range strings.Split(terminaltext.CleanBlock(item.thread.Comments[0].DiffHunk), "\n") {
					body = append(body, ansi.Truncate(line, max(1, width-2), "…"))
				}
			}
		} else {
			body = append(body, comment.Header(view.theme, view.now(), item.comment))
			if selected {
				if text := comment.Body(view.theme, view.markdownRenderer, item.comment, max(1, width-2)); text != "" {
					body = append(body, strings.Split(text, "\n")...)
				}
			} else if strings.TrimSpace(item.comment.Body) != "" {
				body = append(body, "  "+ansi.Truncate(terminaltext.Clean(strings.Join(strings.Fields(item.comment.Body), " ")), max(1, width-4), "…"))
			}
		}
		for j, line := range body {
			if matchingComment >= 0 && j < len(owners) && owners[j] == matchingComment && pane.matchingCommentStart < 0 {
				pane.matchingCommentStart = len(lines)
			}
			prefix := "  "
			if j == 0 {
				prefix = lead
			}
			if selected && j == 0 {
				line = view.theme.Selected.Render(ansi.Strip(line))
			}
			add(prefix + line)
		}
		add("")
	}
	if len(pane.items) == 0 && !view.threadPending && !view.pullRequestPending && view.threadErr == nil && view.detailErr == nil {
		empty := "No comments yet. Press C to start the conversation."
		if pane.unresolved {
			empty = "No unresolved threads. :comments all shows every conversation."
		}
		add(view.theme.Muted.Render(ansi.Wrap(empty, max(1, width), "")))
	}
	offset := pane.viewport.YOffset() - pane.renderedStart
	pane.viewport.SetContent(strings.Join(lines, "\n"))
	if pane.window.Cursor() < len(pane.starts) {
		start := pane.starts[pane.window.Cursor()]
		if pane.renderedSelected == pane.selected {
			pane.viewport.SetYOffset(start + offset)
		} else {
			pane.viewport.SetYOffset(pane.conversationTop(pane.window.Cursor()))
		}
		pane.renderedSelected, pane.renderedStart = pane.selected, start
	}
	return append(header, strings.Split(pane.viewport.View(), "\n")...)
}
