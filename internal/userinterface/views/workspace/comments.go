package workspace

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/comment"
	"github.com/MikePors/strigidae/internal/userinterface/components/scroll"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/components/thread"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// conversationItem retains the source identity, independent of the cursor row.
type conversationItem struct {
	identifier, URL, text string
	comment               comment.Comment
	thread                *github.ReviewThread
	at                    time.Time
	group                 int // drafts, published, local
}

type commentsPane struct {
	dirty                bool
	initialized          bool
	viewport             viewport.Model
	search               search.State
	items                []conversationItem
	window               scroll.Window
	selected             string
	unresolved           bool
	starts               []int
	total, open, drafts  int
	renderedSelected     string
	renderedStart        int
	matchingCommentStart int
}

func (view *View) syncComments() {
	pane := &view.comments
	if pane.initialized && !pane.dirty {
		return
	}
	pane.initialized, pane.dirty = true, false
	var items []conversationItem
	pane.total, pane.open, pane.drafts = 0, 0, 0
	if view.detail != nil {
		for _, currentComment := range view.detail.Comments {
			items = append(items, conversationItem{
				identifier: "comment:" + currentComment.Identifier, URL: currentComment.URL, at: currentComment.CreatedAt, group: 1,
				comment: comment.Comment{Author: currentComment.Author, Action: "commented", At: currentComment.CreatedAt, Body: currentComment.Body, Reactions: currentComment.Reactions},
			})
		}
		for _, review := range view.detail.Reviews {
			if review.State == github.ReviewCommented && strings.TrimSpace(review.Body) == "" {
				continue
			}
			pending := review.State == github.ReviewPending || review.SubmittedAt.IsZero()
			// Empty pending wrappers are represented by their draft threads.
			if pending && strings.TrimSpace(review.Body) == "" {
				continue
			}
			group := 1
			if pending {
				group = 0
			}
			items = append(items, conversationItem{
				identifier: "review:" + review.Identifier, URL: review.URL, at: review.SubmittedAt, group: group,
				comment: comment.Comment{Author: review.Author, Action: reviewAction(review.State), At: review.SubmittedAt, Body: review.Body, Pending: pending},
			})
		}
	}
	for _, reviewThread := range view.paneThreads() {
		item := conversationItem{identifier: "thread:" + reviewThread.Identifier, thread: &reviewThread, group: 1}
		onlyDrafts := len(reviewThread.Comments) > 0
		for _, currentComment := range reviewThread.Comments {
			if currentComment.CreatedAt.After(item.at) {
				item.at = currentComment.CreatedAt
			}
			if !currentComment.Pending {
				onlyDrafts = false
			}
			if currentComment.Pending && !reviewThread.Local.IsLocal() {
				pane.drafts++
			}
			if item.URL == "" {
				item.URL = currentComment.URL
			}
		}
		if onlyDrafts {
			item.group = 0
		}
		if reviewThread.Local.IsLocal() {
			item.group = 2
		} else if !reviewThread.IsResolved {
			pane.open++
		}
		items = append(items, item)
	}
	slices.SortStableFunc(items, func(first, second conversationItem) int {
		if first.group != second.group {
			return first.group - second.group
		}
		if comparison := second.at.Compare(first.at); comparison != 0 {
			return comparison
		}
		return strings.Compare(first.identifier, second.identifier)
	})
	filtered := make([]conversationItem, 0, len(items))
	for _, item := range items {
		if item.group != 2 {
			pane.total++
		}
		if pane.unresolved && (item.thread == nil || item.thread.Local.IsLocal() || item.thread.IsResolved) {
			continue
		}
		text := item.comment.Author + " " + item.comment.Action + " " + item.comment.Body
		if item.comment.Pending {
			text += " draft"
		}
		if item.thread != nil {
			reviewThread := item.thread
			text = threadSearchText(*reviewThread)
			for _, currentComment := range reviewThread.Comments {
				text += " " + currentComment.Author + " " + currentComment.Body
				if currentComment.Pending {
					text += " draft"
				}
			}
		}
		item.text = terminaltext.CleanBlock(text)
		filtered = append(filtered, item)
	}
	pane.items = filtered
	pane.window.SetRows(len(filtered))
	pane.window.SetSize(1)
	if i := slices.IndexFunc(filtered, func(item conversationItem) bool { return item.identifier == pane.selected }); i >= 0 {
		pane.window.GotoRow(i)
	}
	if len(filtered) > 0 {
		pane.selected = filtered[pane.window.Cursor()].identifier
	} else {
		pane.selected = ""
	}
	pane.search.Refresh(view.commentsSearchList())
}

func threadSearchText(reviewThread github.ReviewThread) string {
	text := reviewThread.Path
	switch {
	case reviewThread.Local.IsLocal():
		switch reviewThread.Local {
		case github.LocalWaiting:
			text += " local waiting"
		case github.LocalAnswered:
			text += " local answered"
		case github.LocalFailed:
			text += " local failed"
		}
	case reviewThread.IsResolved:
		text += " resolved"
	default:
		text += " unresolved"
	}
	if reviewThread.IsOutdated {
		text += " outdated"
	}
	return text
}

func (view *View) selectedConversation() *conversationItem {
	view.syncComments()
	if len(view.comments.items) == 0 {
		return nil
	}
	return &view.comments.items[view.comments.window.Cursor()]
}

func checkCommentsArguments(arguments string) tea.Cmd {
	switch strings.TrimSpace(arguments) {
	case "", "all", "unresolved":
		return nil
	default:
		return userinterface.Error(fmt.Errorf("usage: :comments [all|unresolved]"))
	}
}

func (view *View) openComments(arguments string) tea.Cmd {
	switch strings.TrimSpace(arguments) {
	case "":
	case "all":
		view.comments.unresolved = false
	case "unresolved":
		view.comments.unresolved = true
	}
	view.comments.dirty = true
	view.selectEntry(commentsEntry)
	view.focus = paneRight
	view.syncComments()
	return nil
}

func (view *View) commentsSearchList() search.List {
	return search.List{
		Items: func() []search.Item {
			items := make([]search.Item, len(view.comments.items))
			for i, item := range view.comments.items {
				items[i] = search.Item{Identity: item.identifier, Text: item.text}
			}
			return items
		},
		Cursor: func() int { return view.comments.window.Cursor() },
		GoTo:   func(row int) tea.Cmd { view.selectConversation(row, true); return nil },
	}
}

func (view *View) selectConversation(row int, match bool) {
	pane := &view.comments
	if len(pane.items) == 0 {
		return
	}
	pane.window.GotoRow(row)
	pane.selected = pane.items[pane.window.Cursor()].identifier
	_, rightWidth := view.paneWidths()
	view.renderComments(max(1, rightWidth-2), max(1, view.panesHeight()-2))
	if pane.window.Cursor() < len(pane.starts) {
		start := pane.starts[pane.window.Cursor()]
		pane.viewport.SetYOffset(start)
		if match && pane.search.Active() {
			if pane.matchingCommentStart >= 0 {
				pane.viewport.SetYOffset(pane.matchingCommentStart)
			}
			lines := strings.Split(pane.viewport.GetContent(), "\n")
			end := len(lines)
			if pane.window.Cursor()+1 < len(pane.starts) {
				end = pane.starts[pane.window.Cursor()+1]
			}
			for i := start; i < end; i++ {
				if len(search.Match(pane.search.Query(), []string{ansi.Strip(lines[i])})) > 0 {
					pane.viewport.SetYOffset(i)
					break
				}
			}
		}
	}
}

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
			pane.viewport.SetYOffset(start)
		}
		pane.renderedSelected, pane.renderedStart = pane.selected, start
	}
	return append(header, strings.Split(pane.viewport.View(), "\n")...)
}

func (view *View) commentsKey(message keys.Message) (tea.Cmd, bool) {
	view.syncComments()
	pane := &view.comments
	switch {
	case key.Matches(message, view.globalKeys.Search):
		view.searchComments = true
		return pane.search.Start(view.commentsSearchList()), true
	case key.Matches(message, view.globalKeys.NextMatch):
		return pane.search.Step(message, true, view.commentsSearchList(), view.options.WrapScan()), true
	case key.Matches(message, view.globalKeys.PreviousMatch):
		return pane.search.Step(message, false, view.commentsSearchList(), view.options.WrapScan()), true
	case key.Matches(message, view.globalKeys.Cancel):
		pane.search.Clear()
	case key.Matches(message, view.globalKeys.Down):
		view.selectConversation(pane.window.Cursor()+message.Repeat(), false)
	case key.Matches(message, view.globalKeys.Up):
		view.selectConversation(pane.window.Cursor()-message.Repeat(), false)
	case key.Matches(message, view.globalKeys.Top):
		view.selectConversation(0, false)
	case key.Matches(message, view.globalKeys.Bottom):
		row := len(pane.items) - 1
		if message.Count > 0 {
			row = message.Count - 1
		}
		view.selectConversation(row, false)
	case key.Matches(message, view.globalKeys.HalfDown):
		message.Each(pane.viewport.YOffset, pane.viewport.HalfPageDown)
	case key.Matches(message, view.globalKeys.HalfUp):
		message.Each(pane.viewport.YOffset, pane.viewport.HalfPageUp)
	case key.Matches(message, view.globalKeys.PageDown):
		message.Each(pane.viewport.YOffset, pane.viewport.PageDown)
	case key.Matches(message, view.globalKeys.PageUp):
		message.Each(pane.viewport.YOffset, pane.viewport.PageUp)
	case key.Matches(message, view.globalKeys.Select):
		return view.jumpConversation(), true
	case key.Matches(message, view.globalKeys.Open):
		if item := view.selectedConversation(); item != nil && item.URL != "" {
			return view.openAddress(item.URL), true
		}
		return userinterface.Info("no GitHub permalink for this conversation"), true
	default:
		return nil, false
	}
	return nil, true
}

func (view *View) jumpConversation() tea.Cmd {
	item := view.selectedConversation()
	if item == nil || item.thread == nil {
		return nil
	}
	reviewThread := *item.thread
	if view.diffPending || view.diff.Parsed == nil || view.diff.Err != nil {
		return userinterface.Info("diff not loaded")
	}
	if view.detail != nil && view.diffHead != "" && view.detail.HeadSHA != "" && view.diffHead != view.detail.HeadSHA {
		return userinterface.Info("diff is stale; press R to refresh before jumping")
	}
	if reviewThread.IsOutdated {
		return userinterface.Info("No current diff location; press o to open on GitHub.")
	}
	for i, file := range view.files() {
		if file.Path != reviewThread.Path {
			continue
		}
		// Select hidden files explicitly, as the file picker does. Their viewed
		// state stays intact and the right pane's title identifies the destination.
		previous := view.current
		view.selectEntry(i + 1)
		if !view.filePane.JumpThread(reviewThread.Identifier) {
			view.selectEntry(previous)
			return userinterface.Info("No current diff location; press o to open on GitHub.")
		}
		view.focus = paneRight
		return nil
	}
	return userinterface.Info("No current diff location; press o to open on GitHub.")
}
