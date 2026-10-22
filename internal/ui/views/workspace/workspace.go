// Package workspace is the PR workspace: one screen per pull request made of
// tiled panes. The left Files pane lists an Overview entry and the changed
// files; the right pane shows the PR description and conversation, or the
// selected file's diff.
package workspace

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/components/markdown"
	"github.com/MikePors/strigidae/internal/ui/components/picker"
	"github.com/MikePors/strigidae/internal/ui/components/scroll"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// pane identifies which pane has focus.
type pane int

const (
	paneFiles pane = iota
	paneRight
)

// Option configures a View.
type Option func(*View)

// WithClock injects the clock used for relative times.
func WithClock(now func() time.Time) Option { return func(v *View) { v.now = now } }

// WithTheme sets the initial theme. Without it the default dark theme is used
// until the root sends a ui.ThemeMsg; a view that pushes a workspace should
// pass its own current theme.
func WithTheme(th theme.Theme) Option { return func(v *View) { v.th = th } }

// WithOpener replaces how `o` opens the pull request URL, which by default is
// the user's browser.
func WithOpener(open func(url string) error) Option { return func(v *View) { v.open = open } }

// WithFilePane replaces the right pane's content for a selected file; this is
// the hook for the diff component.
func WithFilePane(p FilePane) Option { return func(v *View) { v.filePane = p } }

// View is the PR workspace screen.
type View struct {
	client github.Client
	repo   github.Repo
	number int

	now      func() time.Time
	open     func(url string) error
	th       theme.Theme
	km       keys.Global
	bind     bindings
	md       *markdown.Renderer
	filePane FilePane
	edit     EditFunc

	seq         int // latest load request; older results are stale
	prPending   bool
	diffPending bool
	thrPending  bool
	thrLoaded   bool                  // a thread load has succeeded
	thrErr      error                 // why the last thread load failed; cleared when a load starts
	threads     []github.ReviewThread // sorted by file and line
	detail      *github.PullRequestDetail
	diff        DiffState
	err         error // load failure while there is nothing to show
	notFound    bool

	cseq   int               // latest compose request; older results are stale
	drafts map[string]string // unposted bodies, by thread ID or topLevelDraft

	fw     scroll.Window // Files pane: row 0 is Overview, row i is Files[i-1]
	focus  pane
	ov     overview
	picker *picker.Model // non-nil while the file picker is open
	// pickerItems are the paths the picker was built from; a choice is
	// resolved by path because a refresh may replace the file list meanwhile.
	pickerItems []string
	pickerPaths []string           // raw paths, parallel to pickerItems
	paneFile    github.ChangedFile // last value given to filePane.SetFile
	paneW       int                // size last given to filePane.SetSize
	paneH       int

	width, height int
}

var (
	_ ui.View        = (*View)(nil)
	_ ui.Loader      = (*View)(nil)
	_ ui.KeyCapturer = (*View)(nil)
)

// New returns the workspace for pull request number in repo.
func New(client github.Client, repo github.Repo, number int, opts ...Option) *View {
	v := &View{
		client: client,
		repo:   repo,
		number: number,
		now:    time.Now,
		open:   openInBrowser,
		th:     theme.Default(),
		km:     keys.DefaultGlobal(),
		bind:   defaultBindings(),
		edit:   execEditor,
		drafts: map[string]string{},
	}
	for _, o := range opts {
		o(v)
	}
	v.md = markdown.New(v.th)
	v.ov = newOverview()
	if v.filePane == nil {
		v.filePane = newDiffPane(v.th, func() time.Time { return v.now() })
	}
	v.filePane.SetTheme(v.th)
	return v
}

// Title implements ui.View.
func (v *View) Title() string { return fmt.Sprintf("#%d", v.number) }

// Loading implements ui.Loader.
func (v *View) Loading() bool { return v.prPending || v.diffPending || v.thrPending }

// CapturesKeys implements ui.KeyCapturer: the file picker owns the keyboard
// while it is open.
func (v *View) CapturesKeys() bool { return v.picker != nil }

// Init implements ui.View.
func (v *View) Init() tea.Cmd { return v.load() }

// Resumed implements ui.Resumer. Nothing to do yet: the workspace refreshes
// only on R.
func (v *View) Resumed() tea.Cmd { return nil }

// ShortHelp implements ui.View.
func (v *View) ShortHelp() []key.Binding {
	return []key.Binding{v.bind.PaneLeft, v.bind.PaneRight, v.bind.FilePicker, v.km.Open}
}

// FullHelp implements ui.View. It also registers every multi-key sequence the
// workspace and its file pane react to.
func (v *View) FullHelp() [][]key.Binding {
	out := [][]key.Binding{
		{v.bind.PaneLeft, v.bind.PaneRight, v.bind.NextFile, v.bind.PrevFile, v.bind.FilePicker},
		{v.bind.NextSection, v.bind.PrevSection, v.bind.Comment, v.bind.Reply},
	}
	return append(out, v.filePane.FullHelp()...)
}

// Update implements ui.View.
func (v *View) Update(msg tea.Msg) (ui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
		v.layout()
	case ui.ThemeMsg:
		v.th = msg.Theme
		v.md = markdown.New(v.th)
		v.ov.invalidate()
		v.filePane.SetTheme(v.th)
		v.layout()
	case prLoadedMsg:
		if !v.mine(msg.owner, msg.repo, msg.number, msg.seq) {
			return v, nil
		}
		return v, v.prLoaded(msg)
	case diffLoadedMsg:
		if !v.mine(msg.owner, msg.repo, msg.number, msg.seq) {
			return v, nil
		}
		return v, v.diffLoaded(msg)
	case threadsLoadedMsg:
		if !v.mine(msg.owner, msg.repo, msg.number, msg.seq) {
			return v, nil
		}
		return v, v.threadsLoaded(msg)
	case editedMsg:
		if !v.composeMine(msg.owner, msg.repo, msg.number, msg.seq) {
			return v, nil
		}
		return v, v.edited(msg)
	case postedMsg:
		// A post is never superseded by a later compose, so it is identified
		// by owner, repo and number only.
		if msg.owner != v || msg.repo != v.repo || msg.number != v.number {
			return v, nil
		}
		return v, v.posted(msg)
	case keys.Msg:
		return v, v.handleKey(msg)
	case tea.KeyPressMsg:
		if v.picker != nil {
			v.pickerKey(msg)
		}
	}
	return v, nil
}
