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
	NextHunk, PrevHunk              key.Binding
	NextThread, PrevThread          key.Binding
	ToggleThread                    key.Binding
	OpenThread, CloseThread         key.Binding
	ScrollLeft, ScrollRight         key.Binding
	HalfScrollLeft, HalfScrollRight key.Binding
}

func defaultDiffBindings() diffBindings {
	b := func(k, helpKey, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(helpKey, desc))
	}
	return diffBindings{
		NextHunk:        b("]", "]", "next hunk"),
		PrevHunk:        b("[", "[", "previous hunk"),
		NextThread:      b(")", ")", "next comment thread"),
		PrevThread:      b("(", "(", "previous comment thread"),
		ToggleThread:    b("z a", "za", "toggle comment thread (also enter on a marked line)"),
		OpenThread:      b("z o", "zo", "open comment thread"),
		CloseThread:     b("z c", "zc", "close comment thread"),
		ScrollLeft:      b("z h", "zh", "scroll left"),
		ScrollRight:     b("z l", "zl", "scroll right"),
		HalfScrollLeft:  b("z H", "zH", "scroll half a screen left"),
		HalfScrollRight: b("z L", "zL", "scroll half a screen right"),
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

func newDiffPane(th theme.Theme, now func() time.Time) *diffPane {
	return &diffPane{dv: diffview.New(th, diffview.WithClock(now)), km: keys.DefaultGlobal(), bind: defaultDiffBindings()}
}

func (p *diffPane) SetTheme(th theme.Theme) { p.dv.SetTheme(th) }

func (p *diffPane) SetThreads(ts []github.ReviewThread) { p.dv.SetThreads(ts) }

func (p *diffPane) SetSize(w, h int) { p.dv.SetSize(w, h) }

func (p *diffPane) SetDiff(d DiffState) {
	p.state = d
	p.show()
}

func (p *diffPane) SetFile(f github.ChangedFile) {
	p.file, p.hasFile = f, true
	p.show()
}

// show points the diff component at what the pane should display now.
// diffview ignores a repeated *diff.File and keeps its position when the same
// path is re-parsed, so calling this on every change is cheap and safe.
func (p *diffPane) show() {
	switch {
	case p.state.Err != nil:
		p.dv.SetDiffError(p.state.Err)
	case p.state.Parsed == nil:
		p.dv.SetMessage("Loading diff…")
	case !p.hasFile:
		p.dv.SetFile(nil)
	default:
		f := p.state.Parsed.File(p.file.Path)
		if f == nil {
			msg := "No diff available for this file."
			if p.state.ParseErr != nil {
				msg += "\nThe diff could not be fully parsed: " + p.state.ParseErr.Error()
			}
			p.dv.SetMessage(msg)
			return
		}
		p.dv.SetFile(f)
	}
}

func (p *diffPane) Update(msg keys.Msg) tea.Cmd {
	dv := p.dv
	switch {
	case key.Matches(msg, p.km.Down):
		dv.CursorDown(1)
	case key.Matches(msg, p.km.Up):
		dv.CursorUp(1)
	case key.Matches(msg, p.km.Top):
		dv.GotoTop()
	case key.Matches(msg, p.km.Bottom):
		dv.GotoBottom()
	case key.Matches(msg, p.km.HalfDown):
		dv.HalfPageDown()
	case key.Matches(msg, p.km.HalfUp):
		dv.HalfPageUp()
	case key.Matches(msg, p.km.PageDown):
		dv.PageDown()
	case key.Matches(msg, p.km.PageUp):
		dv.PageUp()
	case key.Matches(msg, p.bind.NextHunk):
		dv.NextHunk()
	case key.Matches(msg, p.bind.PrevHunk):
		dv.PrevHunk()
	case key.Matches(msg, p.bind.NextThread):
		dv.NextThread()
	case key.Matches(msg, p.bind.PrevThread):
		dv.PrevThread()
	case key.Matches(msg, p.bind.ToggleThread), key.Matches(msg, p.km.Select):
		dv.ToggleThread()
	case key.Matches(msg, p.bind.OpenThread):
		dv.OpenThread()
	case key.Matches(msg, p.bind.CloseThread):
		dv.CloseThread()
	case key.Matches(msg, p.bind.ScrollLeft):
		dv.ScrollLeft(hScrollStep)
	case key.Matches(msg, p.bind.ScrollRight):
		dv.ScrollRight(hScrollStep)
	case key.Matches(msg, p.bind.HalfScrollLeft):
		dv.HalfScrollLeft()
	case key.Matches(msg, p.bind.HalfScrollRight):
		dv.HalfScrollRight()
	}
	return nil
}

func (p *diffPane) FocusedThreads() []github.ReviewThread { return p.dv.ThreadsAtCursor() }

func (p *diffPane) CommentRange() (LineRange, bool) {
	r, ok := p.dv.Anchor()
	if !ok {
		return LineRange{}, false
	}
	side := github.SideRight
	if r.Side == diffview.SideLeft {
		side = github.SideLeft
	}
	return LineRange{Path: r.Path, Side: side, Start: r.Start, End: r.End}, true
}

func (p *diffPane) VisualStart() bool          { return p.dv.VisualStart() }
func (p *diffPane) VisualActive() bool         { return p.dv.VisualActive() }
func (p *diffPane) VisualMove(delta int) error { return p.dv.VisualMove(delta) }
func (p *diffPane) VisualEnd()                 { p.dv.VisualEnd() }

func (p *diffPane) Render(width, height int) string { return p.dv.Render(width, height) }

func (p *diffPane) FullHelp() [][]key.Binding {
	b := p.bind
	return [][]key.Binding{{b.NextHunk, b.PrevHunk, b.NextThread, b.PrevThread}, {b.ToggleThread, b.OpenThread, b.CloseThread}, {b.ScrollLeft, b.ScrollRight, b.HalfScrollLeft, b.HalfScrollRight}}
}
