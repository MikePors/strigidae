package workspace

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// hScrollStep is how many columns `z h` / `z l` scroll.
const hScrollStep = 4

// diffBindings are the diff pane's own keys; the movement keys come from
// keys.Global.
type diffBindings struct {
	NextHunk, PreviousHunk          key.Binding
	NextThread, PreviousThread      key.Binding
	ToggleThread                    key.Binding
	OpenThread, CloseThread         key.Binding
	ScrollLeft, ScrollRight         key.Binding
	HalfScrollLeft, HalfScrollRight key.Binding
}

func defaultDiffBindings() diffBindings {
	bind := func(k, helpKey, description string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(helpKey, description))
	}
	return diffBindings{
		NextHunk:        bind("]", "]", "next hunk"),
		PreviousHunk:    bind("[", "[", "previous hunk"),
		NextThread:      bind(")", ")", "next comment thread"),
		PreviousThread:  bind("(", "(", "previous comment thread"),
		ToggleThread:    bind("z a", "za", "toggle comment thread (also enter on a marked line)"),
		OpenThread:      bind("z o", "zo", "open comment thread"),
		CloseThread:     bind("z c", "zc", "close comment thread"),
		ScrollLeft:      bind("z h", "zh", "scroll left"),
		ScrollRight:     bind("z l", "zl", "scroll right"),
		HalfScrollLeft:  bind("z H", "zH", "scroll half a screen left"),
		HalfScrollRight: bind("z L", "zL", "scroll half a screen right"),
	}
}

// diffPane adapts diffview.Model to FilePane: it resolves the selected file in
// the parsed diff and maps keys to diffview's methods.
type diffPane struct {
	diffView   *diffview.Model
	globalKeys keys.Global
	bind       diffBindings

	state   DiffState
	file    github.ChangedFile
	hasFile bool
}

var _ FilePane = (*diffPane)(nil)

func newDiffPane(theme theme.Theme, now func() time.Time) *diffPane {
	return &diffPane{diffView: diffview.New(theme, diffview.WithClock(now)), globalKeys: keys.DefaultGlobal(), bind: defaultDiffBindings()}
}

func (pane *diffPane) SetTheme(theme theme.Theme) { pane.diffView.SetTheme(theme) }

func (pane *diffPane) SetThreads(threads []github.ReviewThread) { pane.diffView.SetThreads(threads) }

func (pane *diffPane) SetSize(width, height int) { pane.diffView.SetSize(width, height) }

func (pane *diffPane) SetDiff(diffState DiffState) {
	pane.state = diffState
	pane.show()
}

func (pane *diffPane) SetFile(file github.ChangedFile) {
	pane.file, pane.hasFile = file, true
	pane.show()
}

// show points the diff component at what the pane should display now.
// diffview ignores a repeated *diff.File and keeps its position when the same
// path is re-parsed, so calling this on every change is cheap and safe.
func (pane *diffPane) show() {
	switch {
	case pane.state.Err != nil:
		pane.diffView.SetDiffError(pane.state.Err)
	case pane.state.Parsed == nil:
		pane.diffView.SetMessage("Loading diff…")
	case !pane.hasFile:
		pane.diffView.SetFile(nil)
	default:
		file := pane.state.Parsed.File(pane.file.Path)
		if file == nil {
			message := "No diff available for this file."
			if pane.state.ParseError != nil {
				message += "\nThe diff could not be fully parsed: " + pane.state.ParseError.Error()
			}
			pane.diffView.SetMessage(message)
			return
		}
		pane.diffView.SetFile(file)
	}
}

func (pane *diffPane) Update(message keys.Message) tea.Cmd {
	diffView := pane.diffView
	switch {
	case key.Matches(message, pane.globalKeys.Down):
		diffView.CursorDown(1)
	case key.Matches(message, pane.globalKeys.Up):
		diffView.CursorUp(1)
	case key.Matches(message, pane.globalKeys.Top):
		diffView.GotoTop()
	case key.Matches(message, pane.globalKeys.Bottom):
		diffView.GotoBottom()
	case key.Matches(message, pane.globalKeys.HalfDown):
		diffView.HalfPageDown()
	case key.Matches(message, pane.globalKeys.HalfUp):
		diffView.HalfPageUp()
	case key.Matches(message, pane.globalKeys.PageDown):
		diffView.PageDown()
	case key.Matches(message, pane.globalKeys.PageUp):
		diffView.PageUp()
	case key.Matches(message, pane.bind.NextHunk):
		diffView.NextHunk()
	case key.Matches(message, pane.bind.PreviousHunk):
		diffView.PreviousHunk()
	case key.Matches(message, pane.bind.NextThread):
		diffView.NextThread()
	case key.Matches(message, pane.bind.PreviousThread):
		diffView.PreviousThread()
	case key.Matches(message, pane.bind.ToggleThread), key.Matches(message, pane.globalKeys.Select):
		diffView.ToggleThread()
	case key.Matches(message, pane.bind.OpenThread):
		diffView.OpenThread()
	case key.Matches(message, pane.bind.CloseThread):
		diffView.CloseThread()
	case key.Matches(message, pane.bind.ScrollLeft):
		diffView.ScrollLeft(hScrollStep)
	case key.Matches(message, pane.bind.ScrollRight):
		diffView.ScrollRight(hScrollStep)
	case key.Matches(message, pane.bind.HalfScrollLeft):
		diffView.HalfScrollLeft()
	case key.Matches(message, pane.bind.HalfScrollRight):
		diffView.HalfScrollRight()
	}
	return nil
}

func (pane *diffPane) FocusedThreads() []github.ReviewThread { return pane.diffView.ThreadsAtCursor() }

func (pane *diffPane) CommentRange() (LineRange, bool) {
	lineRange, ok := pane.diffView.Anchor()
	if !ok {
		return LineRange{}, false
	}
	side := github.SideRight
	if lineRange.Side == diffview.SideLeft {
		side = github.SideLeft
	}
	return LineRange{Path: lineRange.Path, Side: side, Start: lineRange.Start, End: lineRange.End}, true
}

func (pane *diffPane) VisualStart() bool          { return pane.diffView.VisualStart() }
func (pane *diffPane) VisualActive() bool         { return pane.diffView.VisualActive() }
func (pane *diffPane) VisualMove(delta int) error { return pane.diffView.VisualMove(delta) }
func (pane *diffPane) VisualEnd()                 { pane.diffView.VisualEnd() }

func (pane *diffPane) Render(width, height int) string { return pane.diffView.Render(width, height) }

func (pane *diffPane) FullHelp() [][]key.Binding {
	bindings := pane.bind
	return [][]key.Binding{{bindings.NextHunk, bindings.PreviousHunk, bindings.NextThread, bindings.PreviousThread}, {bindings.ToggleThread, bindings.OpenThread, bindings.CloseThread}, {bindings.ScrollLeft, bindings.ScrollRight, bindings.HalfScrollLeft, bindings.HalfScrollRight}}
}
