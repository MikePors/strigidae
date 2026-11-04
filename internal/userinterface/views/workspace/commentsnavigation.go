package workspace

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

func (view *View) selectedConversation() *conversationItem {
	view.syncComments()
	if len(view.comments.items) == 0 {
		return nil
	}
	return &view.comments.items[view.comments.window.Cursor()]
}

// conversationTop includes the group heading when its first item is selected.
func (pane *commentsPane) conversationTop(row int) int {
	start := pane.starts[row]
	if row == 0 || pane.items[row-1].group != pane.items[row].group {
		return max(0, start-1)
	}
	return start
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
		pane.viewport.SetYOffset(pane.conversationTop(pane.window.Cursor()))
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
