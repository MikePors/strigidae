package workspace

import (
	"slices"
	"strings"
	"time"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/comment"
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

			if pending && strings.TrimSpace(review.Body) == "" {
				continue
			}
			group := 1
			if pending {
				group = 0
				pane.drafts++
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
