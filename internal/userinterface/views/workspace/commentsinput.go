package workspace

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

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
