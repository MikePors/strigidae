// Package placeholder is a temporary welcome screen that exercises the app
// shell: a scrollable list, push/pop, loading and status messages. The PR list
// view replaces it.
package placeholder

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

const lineCount = 100

// refreshedMsg is the result of a refresh. Like every real result message it
// carries identity (the screen's depth and a request number) so a screen
// ignores results meant for another screen or an older request.
type refreshedMsg struct{ depth, seq int }

// View is the placeholder screen.
type View struct {
	th            theme.Theme
	km            keys.Global
	depth         int
	seq           int  // latest refresh request
	loading       bool // a refresh is in flight
	cursor, top   int
	width, height int
}

var (
	_ ui.View   = (*View)(nil)
	_ ui.Loader = (*View)(nil)
)

// New returns a placeholder at nesting depth (1 for the root screen).
func New(th theme.Theme, depth int) *View {
	return &View{th: th, km: keys.DefaultGlobal(), depth: depth}
}

// Loading implements ui.Loader.
func (v *View) Loading() bool { return v.loading }

// Init implements ui.View.
func (v *View) Init() tea.Cmd { return nil }

// Title implements ui.View.
func (v *View) Title() string {
	if v.depth <= 1 {
		return "strig"
	}
	return fmt.Sprintf("screen %d", v.depth)
}

// ShortHelp implements ui.View.
func (v *View) ShortHelp() []key.Binding { return []key.Binding{v.km.Select} }

// FullHelp implements ui.View. The placeholder has no view-specific bindings.
func (v *View) FullHelp() [][]key.Binding { return nil }

// Update implements ui.View.
func (v *View) Update(msg tea.Msg) (ui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
		v.clamp()
	case ui.ThemeMsg:
		v.th = msg.Theme
	case refreshedMsg:
		if msg.depth != v.depth || msg.seq != v.seq {
			return v, nil
		}
		v.loading = false
		return v, ui.Info("refreshed (nothing to refresh yet)")
	case keys.Msg:
		return v, v.handleKey(msg)
	}
	return v, nil
}

func (v *View) handleKey(msg keys.Msg) tea.Cmd {
	page := max(1, v.listHeight())
	switch {
	case key.Matches(msg, v.km.Down):
		v.cursor++
	case key.Matches(msg, v.km.Up):
		v.cursor--
	case key.Matches(msg, v.km.Top):
		v.cursor = 0
	case key.Matches(msg, v.km.Bottom):
		v.cursor = lineCount - 1
	case key.Matches(msg, v.km.HalfDown):
		v.cursor += page / 2
	case key.Matches(msg, v.km.HalfUp):
		v.cursor -= page / 2
	case key.Matches(msg, v.km.PageDown):
		v.cursor += page
	case key.Matches(msg, v.km.PageUp):
		v.cursor -= page
	case key.Matches(msg, v.km.Select):
		return ui.Push(New(v.th, v.depth+1))
	case key.Matches(msg, v.km.Refresh):
		v.seq++
		v.loading = true
		depth, seq := v.depth, v.seq
		return tea.Tick(time.Second, func(time.Time) tea.Msg { return refreshedMsg{depth: depth, seq: seq} })
	case key.Matches(msg, v.km.Open):
		return ui.Info("nothing to open in the browser yet")
	case key.Matches(msg, v.km.Cancel):
		return ui.Info("nothing to cancel")
	}
	v.clamp()
	return nil
}

func (v *View) listHeight() int { return max(0, v.height-2) }

// clamp keeps the cursor in range and the window scrolled to show it.
func (v *View) clamp() {
	v.cursor = min(max(v.cursor, 0), lineCount-1)
	h := v.listHeight()
	if h == 0 {
		return
	}
	if v.cursor < v.top {
		v.top = v.cursor
	}
	if v.cursor >= v.top+h {
		v.top = v.cursor - h + 1
	}
	v.top = min(max(v.top, 0), max(0, lineCount-h))
}

// Render implements ui.View.
func (v *View) Render(width, height int) string {
	if height < 2 {
		return ""
	}
	v.width, v.height = width, height
	v.clamp()

	var b strings.Builder
	b.WriteString(v.th.Title.Render(fmt.Sprintf("Welcome to strig (screen %d)", max(1, v.depth))))
	b.WriteString("\n")
	b.WriteString(v.th.Muted.Render("Placeholder: j/k gg/G ctrl-d/u ctrl-f/b to move, enter to push, :q to go back"))
	end := min(lineCount, v.top+v.listHeight())
	for i := v.top; i < end; i++ {
		line := fmt.Sprintf("  Sample item %03d", i+1)
		if i == v.cursor {
			line = v.th.Selected.Render("> " + strings.TrimLeft(line, " "))
		} else {
			line = v.th.Normal.Render(line)
		}
		b.WriteString("\n" + line)
	}
	return b.String()
}
