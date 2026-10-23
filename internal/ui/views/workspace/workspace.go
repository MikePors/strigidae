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
	navBind  diffBindings // hunk/thread keys, routed to the file pane from either focus
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
	diffHead    string // head SHA the diff on screen was loaded for; "" if unknown
	err         error  // load failure while there is nothing to show
	notFound    bool

	cseq int // latest compose request; older results are stale
	// writes counts the comment writes in flight, from the start of a write
	// until its result (or its question) arrives; reviews and merges wait for
	// it, and no second write starts meanwhile.
	writes int
	drafts map[string]string // unposted bodies, by thread ID or topLevelDraft

	viewedState map[string]*viewedToggle // by path; see viewedToggle

	fw     scroll.Window // Files pane: row 0 is Overview, row i is Files[i-1]
	focus  pane
	ov     overview
	picker *picker.Model // non-nil while the file picker is open
	review reviewFlow    // submitting a review (see review.go)
	merge  mergeFlow     // merging the pull request (see merge.go)
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
	_ ui.Commander   = (*View)(nil)
)

// New returns the workspace for pull request number in repo.
func New(client github.Client, repo github.Repo, number int, opts ...Option) *View {
	v := &View{
		client:      client,
		repo:        repo,
		number:      number,
		now:         time.Now,
		open:        openInBrowser,
		th:          theme.Default(),
		km:          keys.DefaultGlobal(),
		bind:        defaultBindings(),
		navBind:     defaultDiffBindings(),
		edit:        execEditor,
		drafts:      map[string]string{},
		viewedState: map[string]*viewedToggle{},
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
func (v *View) Loading() bool {
	return v.prPending || v.diffPending || v.thrPending || v.review.submitting || v.merge.merging
}

// CapturesKeys implements ui.KeyCapturer: the file picker, the verdict
// chooser and a visual-line selection own the keyboard while they are open.
func (v *View) CapturesKeys() bool {
	return v.picker != nil || v.review.chooser != nil || v.visualActive()
}

// visualActive reports whether a line selection is open.
func (v *View) visualActive() bool { return v.filePane.VisualActive() }

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
		{v.bind.NextSection, v.bind.PrevSection, v.bind.Comment, v.bind.Reply, v.bind.Viewed},
		{v.bind.Visual, v.bind.EditComment, v.bind.DeleteComment},
	}
	return append(out, v.filePane.FullHelp()...)
}

// Update implements ui.View.
func (v *View) Update(msg tea.Msg) (ui.View, tea.Cmd) {
	if cmd, ok := v.review.update(msg, v.reviewEnv()); ok {
		return v, cmd
	}
	if cmd, ok := v.merge.update(msg, v.reviewEnv()); ok {
		return v, cmd
	}
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
		cmd := v.prLoaded(msg)
		if msg.thenDiff {
			cmd = tea.Batch(cmd, v.fetchDiff(msg))
		}
		return v, cmd
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
	case viewedMsg:
		if v.viewedMine(msg) {
			return v, v.viewedDone(msg)
		}
	case editedMsg:
		if !v.composeMine(origin{msg.owner, msg.repo, msg.number}, msg.seq) {
			return v, nil
		}
		return v, v.edited(msg)
	case postedMsg:
		// A post is never superseded by a later compose, so it is identified
		// by owner, repo and number only.
		if !v.owns(msg.origin) {
			return v, nil
		}
		return v, v.posted(msg)
	case commentDeletedMsg:
		if !v.owns(msg.origin) {
			return v, nil
		}
		return v, v.commentDeleted(msg)
	case commentCheckedMsg:
		if !v.owns(msg.origin) {
			return v, nil
		}
		v.writeDone() // the write waits for the answer
		return v, ui.Confirm(msg.question, msg.onYes)
	case commentAgreedMsg:
		if !v.owns(msg.origin) {
			return v, nil
		}
		return v, v.commentAgreed(msg)
	case commentReadMsg:
		if !v.owns(msg.origin) || msg.seq != v.cseq { // another editor started meanwhile
			return v, nil
		}
		return v, v.commentRead(msg)
	case keys.Msg:
		return v, v.handleKey(msg)
	case tea.KeyPressMsg:
		if v.visualActive() {
			return v, v.visualKey(msg)
		}
		if v.review.chooser != nil {
			return v, v.review.chooserKey(msg, v.reviewEnv())
		}
		if v.picker != nil {
			v.pickerKey(msg)
		}
	}
	return v, nil
}
