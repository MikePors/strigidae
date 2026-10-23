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
	dv   *diffview.Model
	km   keys.Global
	bind diffBindings

	state   DiffState
	file    github.ChangedFile
	hasFile bool
}

var _ FilePane = (*diffPane)(nil)

func newDiffPane(theme theme.Theme, now func() time.Time) *diffPane {
	return &diffPane{dv: diffview.New(theme, diffview.WithClock(now)), km: keys.DefaultGlobal(), bind: defaultDiffBindings()}
}

func (pane *diffPane) SetTheme(theme theme.Theme) { pane.dv.SetTheme(theme) }

func (pane *diffPane) SetThreads(threads []github.ReviewThread) { pane.dv.SetThreads(threads) }

func (pane *diffPane) SetSize(width, height int) { pane.dv.SetSize(width, height) }

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
		pane.dv.SetDiffError(pane.state.Err)
	case pane.state.Parsed == nil:
		pane.dv.SetMessage("Loading diff…")
	case !pane.hasFile:
		pane.dv.SetFile(nil)
	default:
		file := pane.state.Parsed.File(pane.file.Path)
		if file == nil {
			message := "No diff available for this file."
			if pane.state.ParseError != nil {
				message += "\nThe diff could not be fully parsed: " + pane.state.ParseError.Error()
			}
			pane.dv.SetMessage(message)
			return
		}
		pane.dv.SetFile(file)
	}
}

func (pane *diffPane) Update(message keys.Message) tea.Cmd {
	diffView := pane.dv
	switch {
	case key.Matches(message, pane.km.Down):
		diffView.CursorDown(1)
	case key.Matches(message, pane.km.Up):
		diffView.CursorUp(1)
	case key.Matches(message, pane.km.Top):
		diffView.GotoTop()
	case key.Matches(message, pane.km.Bottom):
		diffView.GotoBottom()
	case key.Matches(message, pane.km.HalfDown):
		diffView.HalfPageDown()
	case key.Matches(message, pane.km.HalfUp):
		diffView.HalfPageUp()
	case key.Matches(message, pane.km.PageDown):
		diffView.PageDown()
	case key.Matches(message, pane.km.PageUp):
		diffView.PageUp()
	case key.Matches(message, pane.bind.NextHunk):
		diffView.NextHunk()
	case key.Matches(message, pane.bind.PreviousHunk):
		diffView.PreviousHunk()
	case key.Matches(message, pane.bind.NextThread):
		diffView.NextThread()
	case key.Matches(message, pane.bind.PreviousThread):
		diffView.PreviousThread()
	case key.Matches(message, pane.bind.ToggleThread), key.Matches(message, pane.km.Select):
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

func (pane *diffPane) FocusedThreads() []github.ReviewThread { return pane.dv.ThreadsAtCursor() }

func (pane *diffPane) CommentRange() (LineRange, bool) {
	lineRange, ok := pane.dv.Anchor()
	if !ok {
		return LineRange{}, false
	}
	side := github.SideRight
	if lineRange.Side == diffview.SideLeft {
		side = github.SideLeft
	}
	return LineRange{Path: lineRange.Path, Side: side, Start: lineRange.Start, End: lineRange.End}, true
}

func (pane *diffPane) VisualStart() bool          { return pane.dv.VisualStart() }
func (pane *diffPane) VisualActive() bool         { return pane.dv.VisualActive() }
func (pane *diffPane) VisualMove(delta int) error { return pane.dv.VisualMove(delta) }
func (pane *diffPane) VisualEnd()                 { pane.dv.VisualEnd() }

func (pane *diffPane) Render(width, height int) string { return pane.dv.Render(width, height) }

func (pane *diffPane) FullHelp() [][]key.Binding {
	bindings := pane.bind
	return [][]key.Binding{{bindings.NextHunk, bindings.PreviousHunk, bindings.NextThread, bindings.PreviousThread}, {bindings.ToggleThread, bindings.OpenThread, bindings.CloseThread}, {bindings.ScrollLeft, bindings.ScrollRight, bindings.HalfScrollLeft, bindings.HalfScrollRight}}
}
