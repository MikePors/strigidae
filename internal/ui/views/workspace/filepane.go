package workspace

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// DiffState is the workspace's raw diff and where loading it stands.
type DiffState struct {
	// Raw is the unified diff of the whole pull request. On a refresh it keeps
	// the previous text while Loading is set.
	Raw string
	// Err is why the diff could not be loaded, for example
	// github.ErrDiffTooLarge. Raw is empty then.
	Err error
	// Loading is set while a (re)load is in flight.
	Loading bool
}

// FilePane is the content of the right pane while a file is selected. The
// diff component plugs in here through WithFilePane.
//
// The workspace calls SetTheme, SetDiff and SetFile outside of rendering,
// forwards every keys.Msg it does not handle itself (movement, scrolling and
// the pane's own sequences) to Update while the right pane has focus, and
// calls Render with the pane's inner size. FullHelp must list every binding
// Update reacts to, so multi-key sequences are recognised.
type FilePane interface {
	// SetTheme is called on construction and whenever the theme changes.
	SetTheme(th theme.Theme)
	// SetDiff is called when a diff load starts and when it finishes.
	SetDiff(d DiffState)
	// SetFile is called when the selected file changes (and when the pull
	// request reloads with a file selected).
	SetFile(f github.ChangedFile)
	// Update handles a key while the right pane has focus.
	Update(msg keys.Msg) tea.Cmd
	// Render draws the pane in exactly width columns and at most height rows.
	Render(width, height int) string
	// FullHelp lists the pane's bindings for the help overlay and sequence matching.
	FullHelp() [][]key.Binding
}

// placeholderPane stands in until the diff component exists.
type placeholderPane struct {
	th   theme.Theme
	file github.ChangedFile
}

var _ FilePane = (*placeholderPane)(nil)

func (p *placeholderPane) SetTheme(th theme.Theme)      { p.th = th }
func (p *placeholderPane) SetDiff(DiffState)            {}
func (p *placeholderPane) SetFile(f github.ChangedFile) { p.file = f }
func (p *placeholderPane) Update(keys.Msg) tea.Cmd      { return nil }
func (p *placeholderPane) FullHelp() [][]key.Binding    { return nil }

func (p *placeholderPane) Render(_, _ int) string {
	f := p.file
	return strings.Join([]string{
		p.th.Title.Render(f.Path),
		p.th.Muted.Render(fmt.Sprintf("%s  +%d -%d", strings.ToLower(string(f.Change)), f.Additions, f.Deletions)),
		"",
		p.th.Muted.Render("diff view arrives with #8"),
	}, "\n")
}
