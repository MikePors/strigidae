package workspace

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// bindings are the keys specific to the workspace. Movement keys come from
// keys.Global.
type bindings struct {
	PaneLeft, PaneRight          key.Binding
	NextFile, PreviousFile       key.Binding
	FilePicker                   key.Binding
	NextSection, PreviousSection key.Binding
	Comment, Reply               key.Binding
	Viewed                       key.Binding
	ReviewHunk                   key.Binding
	ResolveThread                key.Binding
	// Inline comments: V starts a line selection, e and D change your comment
	// at the cursor. The Visual* keys apply while a selection is open.
	Visual, EditComment, DeleteComment key.Binding
	VisualDown, VisualUp, VisualExit   key.Binding
}

func defaultBindings() bindings {
	bind := func(pressed, helpKey, description string) key.Binding {
		return key.NewBinding(key.WithKeys(pressed), key.WithHelp(helpKey, description))
	}
	return bindings{
		PaneLeft:        bind("h", "h", "focus files pane"),
		PaneRight:       bind("l", "l", "focus right pane"),
		NextFile:        bind("}", "}", "next file"),
		PreviousFile:    bind("{", "{", "previous file"),
		FilePicker:      bind("f", "f", "file picker"),
		NextSection:     bind("]", "]", "next section (overview)"),
		PreviousSection: bind("[", "[", "previous section (overview)"),
		Comment:         bind("c", "c", "new comment: on the pull request (overview) or on the diff line / selection"),
		Reply:           bind("r", "r", "reply to the comment thread at the cursor (diff)"),
		ResolveThread:   bind("x", "x", "resolve or unresolve the comment thread at the cursor (diff; also :resolve, :unresolve)"),
		Viewed:          bind("v", "v", "toggle file viewed"),
		ReviewHunk:      bind("m", "m", "toggle hunk reviewed at the diff cursor (kept by strig until it quits)"),

		Visual:        bind("V", "V", "select diff lines for a multi-line comment"),
		EditComment:   bind("e", "e", "edit your comment at the cursor (diff; also :edit-comment)"),
		DeleteComment: bind("D", "D", "delete your comment at the cursor (diff; also :delete-comment)"),
		VisualDown:    key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "extend selection down")),
		VisualUp:      key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "extend selection up")),
		VisualExit:    key.NewBinding(key.WithKeys("esc", "V"), key.WithHelp("esc", "leave visual mode")),
	}
}

// actions names the workspace's own bindings, in the namespace of the global
// ones (keys.Global.Actions). Next and previous section are the dashboard's
// names too: the same action in another view.
func (workspaceBindings *bindings) actions() []keys.Action {
	return []keys.Action{
		{Name: "focus-files-pane", Binding: &workspaceBindings.PaneLeft},
		{Name: "focus-right-pane", Binding: &workspaceBindings.PaneRight},
		{Name: "next-file", Binding: &workspaceBindings.NextFile},
		{Name: "previous-file", Binding: &workspaceBindings.PreviousFile},
		{Name: "file-picker", Binding: &workspaceBindings.FilePicker},
		{Name: "next-section", Binding: &workspaceBindings.NextSection},
		{Name: "previous-section", Binding: &workspaceBindings.PreviousSection},
		{Name: "new-comment", Binding: &workspaceBindings.Comment},
		{Name: "reply", Binding: &workspaceBindings.Reply},
		{Name: "toggle-viewed", Binding: &workspaceBindings.Viewed},
		{Name: "toggle-hunk-reviewed", Binding: &workspaceBindings.ReviewHunk},
		{Name: "resolve-thread", Binding: &workspaceBindings.ResolveThread},
		{Name: "start-visual-mode", Binding: &workspaceBindings.Visual},
		{Name: "edit-comment", Binding: &workspaceBindings.EditComment},
		{Name: "delete-comment", Binding: &workspaceBindings.DeleteComment},
		{Name: "extend-selection-down", Binding: &workspaceBindings.VisualDown},
		{Name: "extend-selection-up", Binding: &workspaceBindings.VisualUp},
		{Name: "leave-visual-mode", Binding: &workspaceBindings.VisualExit},
	}
}

func openInBrowser(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}

func (view *View) handleKey(message keys.Message) tea.Cmd {
	switch {
	case key.Matches(message, view.bind.PaneLeft):
		view.focus = paneFiles
	case key.Matches(message, view.bind.PaneRight):
		view.focus = paneRight
	case key.Matches(message, view.bind.NextFile):
		view.stepFile(message.Repeat())
	case key.Matches(message, view.bind.PreviousFile):
		view.stepFile(-message.Repeat())
	case key.Matches(message, view.bind.FilePicker):
		view.openPicker()
	case view.selection() == 0 && key.Matches(message, view.bind.NextSection):
		message.Each(view.overview.viewport.YOffset, func() { view.jumpSection(1) })
	case view.selection() == 0 && key.Matches(message, view.bind.PreviousSection):
		message.Each(view.overview.viewport.YOffset, func() { view.jumpSection(-1) })
	case key.Matches(message, view.bind.Comment):
		return view.comment()
	case key.Matches(message, view.bind.Viewed):
		return view.toggleViewed()
	case key.Matches(message, view.bind.ReviewHunk):
		return view.toggleHunkReviewed()
	case key.Matches(message, view.bind.Reply):
		return view.reply()
	case key.Matches(message, view.bind.ResolveThread):
		return view.resolveThread(resolveToggle)
	case view.selection() > 0 && key.Matches(message, view.bind.Visual):
		return view.startVisual()
	case key.Matches(message, view.bind.EditComment):
		return view.editComment()
	case key.Matches(message, view.bind.DeleteComment):
		return view.deleteComment()
	case view.focus == paneFiles && key.Matches(message, view.globalKeys.Search):
		return view.search.Start(view.searchList())
	case view.focus == paneFiles && key.Matches(message, view.globalKeys.NextMatch):
		return view.search.Step(message, true, view.searchList(), view.options.WrapScan())
	case view.focus == paneFiles && key.Matches(message, view.globalKeys.PreviousMatch):
		return view.search.Step(message, false, view.searchList(), view.options.WrapScan())
	case view.focus == paneFiles && key.Matches(message, view.globalKeys.Cancel):
		view.search.Clear()
	case key.Matches(message, view.globalKeys.Open):
		return view.openBrowser()
	case key.Matches(message, view.globalKeys.Refresh):
		return view.load()
	case view.selection() > 0 && view.diffNavigation(message):
		// Reserved globally: hunk and thread keys work from either pane.
		return view.filePane.Update(message)
	case view.focus == paneFiles:
		view.filesKey(message)
	default:
		return view.rightKey(message)
	}
	view.layout()
	return nil
}

// stepFile moves to the next or previous file. It never lands on Overview.
func (view *View) stepFile(delta int) {
	if view.selection() == 0 && delta < 0 {
		return
	}
	// Hidden files are skipped: count along the shown file entries.
	var before, after []int
	for _, entry := range view.rows[1:] {
		if entry < view.selection() {
			before = append(before, entry)
		} else if entry > view.selection() {
			after = append(after, entry)
		}
	}
	switch {
	case delta > 0 && len(after) > 0:
		view.selectEntry(after[min(delta, len(after))-1])
	case delta < 0 && len(before) > 0:
		view.selectEntry(before[len(before)-min(-delta, len(before))])
	}
}

func (view *View) openBrowser() tea.Cmd {
	if view.detail == nil || view.detail.URL == "" {
		return userinterface.Info("pull request not loaded yet")
	}
	return view.openAddress(view.detail.URL)
}

// openAddress opens url in the browser through the injected opener.
func (view *View) openAddress(url string) tea.Cmd {
	open := view.open
	return func() tea.Msg {
		if err := open(url); err != nil {
			return userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("open %s in browser: %w", terminaltext.Clean(url), err))}
		}
		return nil
	}
}

// filesKey moves the selection in the Files pane.
func (view *View) filesKey(message keys.Message) {
	previous := view.window.Cursor()
	switch {
	case key.Matches(message, view.globalKeys.Down):
		view.window.Down(message.Repeat())
	case key.Matches(message, view.globalKeys.Up):
		view.window.Up(message.Repeat())
	case key.Matches(message, view.globalKeys.Top):
		view.window.GotoTop()
	case key.Matches(message, view.globalKeys.Bottom):
		if message.Count > 0 {
			view.window.GotoRow(message.Count - 1)
		} else {
			view.window.GotoBottom()
		}
	case key.Matches(message, view.globalKeys.HalfDown):
		message.Each(view.window.Cursor, view.window.HalfPageDown)
	case key.Matches(message, view.globalKeys.HalfUp):
		message.Each(view.window.Cursor, view.window.HalfPageUp)
	case key.Matches(message, view.globalKeys.PageDown):
		message.Each(view.window.Cursor, view.window.PageDown)
	case key.Matches(message, view.globalKeys.PageUp):
		message.Each(view.window.Cursor, view.window.PageUp)
	case key.Matches(message, view.globalKeys.ScrollCursorTop):
		view.window.ScrollCursorTop()
	case key.Matches(message, view.globalKeys.ScrollCursorMiddle):
		view.window.ScrollCursorMiddle()
	case key.Matches(message, view.globalKeys.ScrollCursorBottom):
		view.window.ScrollCursorBottom()
	case key.Matches(message, view.globalKeys.Select):
		view.focus = paneRight
	}
	if view.window.Cursor() != previous {
		view.cursorMoved()
	}
}

// rightKey scrolls the Overview, or hands the key to the file pane.
func (view *View) rightKey(message keys.Message) tea.Cmd {
	if view.selection() > 0 {
		return view.filePane.Update(message)
	}
	viewport := &view.overview.viewport
	switch {
	case key.Matches(message, view.globalKeys.Down):
		viewport.ScrollDown(message.Repeat())
	case key.Matches(message, view.globalKeys.Up):
		viewport.ScrollUp(message.Repeat())
	case key.Matches(message, view.globalKeys.Top):
		viewport.GotoTop()
	case key.Matches(message, view.globalKeys.Bottom):
		if message.Count > 0 {
			viewport.SetYOffset(message.Count - 1)
		} else {
			viewport.GotoBottom()
		}
	case key.Matches(message, view.globalKeys.HalfDown):
		message.Each(viewport.YOffset, viewport.HalfPageDown)
	case key.Matches(message, view.globalKeys.HalfUp):
		message.Each(viewport.YOffset, viewport.HalfPageUp)
	case key.Matches(message, view.globalKeys.PageDown):
		message.Each(viewport.YOffset, viewport.PageDown)
	case key.Matches(message, view.globalKeys.PageUp):
		message.Each(viewport.YOffset, viewport.PageUp)
	}
	return nil
}

// jumpSection moves the Overview to the next or previous section. ] and [ are
// sections only on the Overview; on a file they belong to the diff pane.
func (view *View) jumpSection(delta int) {
	if view.detail == nil {
		return
	}
	view.overview.jump(delta)
}

// diffNavigation reports whether message is a hunk or thread movement key.
func (view *View) diffNavigation(message keys.Message) bool {
	bindings := view.navigationBind
	return key.Matches(message, bindings.NextHunk, bindings.PreviousHunk, bindings.NextThread, bindings.PreviousThread)
}

// ActionNames lists the action names of the workspace and its diff pane, for
// the check that views don't reuse each other's names by accident.
func ActionNames() []string {
	workspaceBindings, paneBindings := defaultBindings(), defaultDiffBindings()
	var names []string
	for _, action := range append(workspaceBindings.actions(), paneBindings.actions()...) {
		names = append(names, action.Name)
	}
	return names
}
