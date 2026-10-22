package workspace

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// bindings are the keys specific to the workspace. Movement keys come from
// keys.Global.
type bindings struct {
	PaneLeft, PaneRight      key.Binding
	NextFile, PrevFile       key.Binding
	FilePicker               key.Binding
	NextSection, PrevSection key.Binding
	Comment, Reply           key.Binding
}

func defaultBindings() bindings {
	b := func(k, helpKey, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(helpKey, desc))
	}
	return bindings{
		PaneLeft:    b("h", "h", "focus files pane"),
		PaneRight:   b("l", "l", "focus right pane"),
		NextFile:    b("}", "}", "next file"),
		PrevFile:    b("{", "{", "previous file"),
		FilePicker:  b("f", "f", "file picker"),
		NextSection: b("]", "]", "next section (overview)"),
		PrevSection: b("[", "[", "previous section (overview)"),
		Comment:     b("c", "c", "new comment on the pull request (overview)"),
		Reply:       b("r", "r", "reply to the comment thread at the cursor (diff)"),
	}
}

func openInBrowser(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}

func (v *View) handleKey(msg keys.Msg) tea.Cmd {
	switch {
	case key.Matches(msg, v.bind.PaneLeft):
		v.focus = paneFiles
	case key.Matches(msg, v.bind.PaneRight):
		v.focus = paneRight
	case key.Matches(msg, v.bind.NextFile):
		v.stepFile(1)
	case key.Matches(msg, v.bind.PrevFile):
		v.stepFile(-1)
	case key.Matches(msg, v.bind.FilePicker):
		v.openPicker()
	case v.sel() == 0 && key.Matches(msg, v.bind.NextSection):
		v.jumpSection(1)
	case v.sel() == 0 && key.Matches(msg, v.bind.PrevSection):
		v.jumpSection(-1)
	case key.Matches(msg, v.bind.Comment):
		return v.comment()
	case key.Matches(msg, v.bind.Reply):
		return v.reply()
	case key.Matches(msg, v.km.Open):
		return v.openBrowser()
	case key.Matches(msg, v.km.Refresh):
		return v.load()
	case v.sel() > 0 && v.diffNav(msg):
		// Reserved globally: hunk and thread keys work from either pane.
		return v.filePane.Update(msg)
	case v.focus == paneFiles:
		v.filesKey(msg)
	default:
		return v.rightKey(msg)
	}
	v.layout()
	return nil
}

// stepFile moves to the next or previous file. It never lands on Overview.
func (v *View) stepFile(delta int) {
	if len(v.files()) == 0 || (v.sel() == 0 && delta < 0) {
		return
	}
	v.selectEntry(min(max(v.sel()+delta, 1), len(v.files())))
}

func (v *View) openBrowser() tea.Cmd {
	if v.detail == nil || v.detail.URL == "" {
		return ui.Info("pull request not loaded yet")
	}
	url, open := v.detail.URL, v.open
	return func() tea.Msg {
		if err := open(url); err != nil {
			return ui.ErrorMsg{Err: termtext.SafeError(fmt.Errorf("open %s in browser: %w", termtext.Clean(url), err))}
		}
		return nil
	}
}

// filesKey moves the selection in the Files pane.
func (v *View) filesKey(msg keys.Msg) {
	prev := v.sel()
	switch {
	case key.Matches(msg, v.km.Down):
		v.fw.Down(1)
	case key.Matches(msg, v.km.Up):
		v.fw.Up(1)
	case key.Matches(msg, v.km.Top):
		v.fw.GotoTop()
	case key.Matches(msg, v.km.Bottom):
		v.fw.GotoBottom()
	case key.Matches(msg, v.km.HalfDown):
		v.fw.HalfPageDown()
	case key.Matches(msg, v.km.HalfUp):
		v.fw.HalfPageUp()
	case key.Matches(msg, v.km.PageDown):
		v.fw.PageDown()
	case key.Matches(msg, v.km.PageUp):
		v.fw.PageUp()
	case key.Matches(msg, v.km.Select):
		v.focus = paneRight
	}
	if v.sel() != prev {
		v.selectionChanged()
	}
}

// rightKey scrolls the Overview, or hands the key to the file pane.
func (v *View) rightKey(msg keys.Msg) tea.Cmd {
	if v.sel() > 0 {
		return v.filePane.Update(msg)
	}
	vp := &v.ov.vp
	switch {
	case key.Matches(msg, v.km.Down):
		vp.ScrollDown(1)
	case key.Matches(msg, v.km.Up):
		vp.ScrollUp(1)
	case key.Matches(msg, v.km.Top):
		vp.GotoTop()
	case key.Matches(msg, v.km.Bottom):
		vp.GotoBottom()
	case key.Matches(msg, v.km.HalfDown):
		vp.HalfPageDown()
	case key.Matches(msg, v.km.HalfUp):
		vp.HalfPageUp()
	case key.Matches(msg, v.km.PageDown):
		vp.PageDown()
	case key.Matches(msg, v.km.PageUp):
		vp.PageUp()
	}
	return nil
}

// jumpSection moves the Overview to the next or previous section. ] and [ are
// sections only on the Overview; on a file they belong to the diff pane.
func (v *View) jumpSection(delta int) {
	if v.detail == nil {
		return
	}
	v.ov.jump(delta)
}

// diffNav reports whether msg is a hunk or thread movement key.
func (v *View) diffNav(msg keys.Msg) bool {
	b := v.navBind
	return key.Matches(msg, b.NextHunk, b.PrevHunk, b.NextThread, b.PrevThread)
}
