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
	// Inline comments: V starts a line selection, e and D change your comment
	// at the cursor. The Visual* keys apply while a selection is open.
	Visual, EditComment, DeleteComment key.Binding
	VisualDown, VisualUp, VisualExit   key.Binding
}

func defaultBindings() bindings {
	bind := func(k, helpKey, description string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(helpKey, description))
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
		Viewed:          bind("v", "v", "toggle file viewed"),

		Visual:        bind("V", "V", "select diff lines for a multi-line comment"),
		EditComment:   bind("e", "e", "edit your comment at the cursor (diff; also :edit-comment)"),
		DeleteComment: bind("D", "D", "delete your comment at the cursor (diff; also :delete-comment)"),
		VisualDown:    key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "extend selection down")),
		VisualUp:      key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "extend selection up")),
		VisualExit:    key.NewBinding(key.WithKeys("esc", "V"), key.WithHelp("esc", "leave visual mode")),
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
		view.stepFile(1)
	case key.Matches(message, view.bind.PreviousFile):
		view.stepFile(-1)
	case key.Matches(message, view.bind.FilePicker):
		view.openPicker()
	case view.sel() == 0 && key.Matches(message, view.bind.NextSection):
		view.jumpSection(1)
	case view.sel() == 0 && key.Matches(message, view.bind.PreviousSection):
		view.jumpSection(-1)
	case key.Matches(message, view.bind.Comment):
		return view.comment()
	case key.Matches(message, view.bind.Viewed):
		return view.toggleViewed()
	case key.Matches(message, view.bind.Reply):
		return view.reply()
	case view.sel() > 0 && key.Matches(message, view.bind.Visual):
		return view.startVisual()
	case key.Matches(message, view.bind.EditComment):
		return view.editComment()
	case key.Matches(message, view.bind.DeleteComment):
		return view.deleteComment()
	case key.Matches(message, view.km.Open):
		return view.openBrowser()
	case key.Matches(message, view.km.Refresh):
		return view.load()
	case view.sel() > 0 && view.diffNav(message):
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
	if len(view.files()) == 0 || (view.sel() == 0 && delta < 0) {
		return
	}
	view.selectEntry(min(max(view.sel()+delta, 1), len(view.files())))
}

func (view *View) openBrowser() tea.Cmd {
	if view.detail == nil || view.detail.URL == "" {
		return userinterface.Info("pull request not loaded yet")
	}
	url, open := view.detail.URL, view.open
	return func() tea.Msg {
		if err := open(url); err != nil {
			return userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("open %s in browser: %w", terminaltext.Clean(url), err))}
		}
		return nil
	}
}

// filesKey moves the selection in the Files pane.
func (view *View) filesKey(message keys.Message) {
	previous := view.sel()
	switch {
	case key.Matches(message, view.km.Down):
		view.fw.Down(1)
	case key.Matches(message, view.km.Up):
		view.fw.Up(1)
	case key.Matches(message, view.km.Top):
		view.fw.GotoTop()
	case key.Matches(message, view.km.Bottom):
		view.fw.GotoBottom()
	case key.Matches(message, view.km.HalfDown):
		view.fw.HalfPageDown()
	case key.Matches(message, view.km.HalfUp):
		view.fw.HalfPageUp()
	case key.Matches(message, view.km.PageDown):
		view.fw.PageDown()
	case key.Matches(message, view.km.PageUp):
		view.fw.PageUp()
	case key.Matches(message, view.km.Select):
		view.focus = paneRight
	}
	if view.sel() != previous {
		view.selectionChanged()
	}
}

// rightKey scrolls the Overview, or hands the key to the file pane.
func (view *View) rightKey(message keys.Message) tea.Cmd {
	if view.sel() > 0 {
		return view.filePane.Update(message)
	}
	viewport := &view.ov.vp
	switch {
	case key.Matches(message, view.km.Down):
		viewport.ScrollDown(1)
	case key.Matches(message, view.km.Up):
		viewport.ScrollUp(1)
	case key.Matches(message, view.km.Top):
		viewport.GotoTop()
	case key.Matches(message, view.km.Bottom):
		viewport.GotoBottom()
	case key.Matches(message, view.km.HalfDown):
		viewport.HalfPageDown()
	case key.Matches(message, view.km.HalfUp):
		viewport.HalfPageUp()
	case key.Matches(message, view.km.PageDown):
		viewport.PageDown()
	case key.Matches(message, view.km.PageUp):
		viewport.PageUp()
	}
	return nil
}

// jumpSection moves the Overview to the next or previous section. ] and [ are
// sections only on the Overview; on a file they belong to the diff pane.
func (view *View) jumpSection(delta int) {
	if view.detail == nil {
		return
	}
	view.ov.jump(delta)
}

// diffNav reports whether msg is a hunk or thread movement key.
func (view *View) diffNav(message keys.Message) bool {
	bindings := view.navBind
	return key.Matches(message, bindings.NextHunk, bindings.PreviousHunk, bindings.NextThread, bindings.PreviousThread)
}
