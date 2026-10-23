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
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/markdown"
	"github.com/MikePors/strigidae/internal/userinterface/components/picker"
	"github.com/MikePors/strigidae/internal/userinterface/components/scroll"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
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
func WithClock(now func() time.Time) Option { return func(view *View) { view.now = now } }

// WithTheme sets the initial theme. Without it the default dark theme is used
// until the root sends a userinterface.ThemeMsg; a view that pushes a workspace should
// pass its own current theme.
func WithTheme(theme theme.Theme) Option { return func(view *View) { view.th = theme } }

// WithOpener replaces how `o` opens the pull request URL, which by default is
// the user's browser.
func WithOpener(open func(url string) error) Option { return func(view *View) { view.open = open } }

// WithFilePane replaces the right pane's content for a selected file; this is
// the hook for the diff component.
func WithFilePane(pane FilePane) Option { return func(view *View) { view.filePane = pane } }

// View is the PR workspace screen.
type View struct {
	client     github.Client
	repository github.Repository
	number     int

	now      func() time.Time
	open     func(url string) error
	th       theme.Theme
	km       keys.Global
	bind     bindings
	navBind  diffBindings // hunk/thread keys, routed to the file pane from either focus
	md       *markdown.Renderer
	filePane FilePane
	edit     EditFunction

	sequence           int // latest load request; older results are stale
	pullRequestPending bool
	diffPending        bool
	thrPending         bool
	thrLoaded          bool                  // a thread load has succeeded
	thrErr             error                 // why the last thread load failed; cleared when a load starts
	threads            []github.ReviewThread // sorted by file and line
	detail             *github.PullRequestDetail
	diff               DiffState
	diffHead           string // head SHA the diff on screen was loaded for; "" if unknown
	err                error  // load failure while there is nothing to show
	notFound           bool

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
	paneWidth   int                // size last given to filePane.SetSize
	paneHeight  int

	width, height int
}

var (
	_ userinterface.View        = (*View)(nil)
	_ userinterface.Loader      = (*View)(nil)
	_ userinterface.KeyCapturer = (*View)(nil)
	_ userinterface.Commander   = (*View)(nil)
)

// New returns the workspace for pull request number in repo.
func New(client github.Client, repository github.Repository, number int, options ...Option) *View {
	view := &View{
		client:      client,
		repository:  repository,
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
	for _, option := range options {
		option(view)
	}
	view.md = markdown.New(view.th)
	view.ov = newOverview()
	if view.filePane == nil {
		view.filePane = newDiffPane(view.th, func() time.Time { return view.now() })
	}
	view.filePane.SetTheme(view.th)
	return view
}

// Title implements userinterface.View.
func (view *View) Title() string { return fmt.Sprintf("#%d", view.number) }

// Loading implements userinterface.Loader.
func (view *View) Loading() bool {
	return view.pullRequestPending || view.diffPending || view.thrPending || view.review.submitting || view.merge.merging
}

// CapturesKeys implements userinterface.KeyCapturer: the file picker, the verdict
// chooser and a visual-line selection own the keyboard while they are open.
func (view *View) CapturesKeys() bool {
	return view.picker != nil || view.review.chooser != nil || view.visualActive()
}

// visualActive reports whether a line selection is open.
func (view *View) visualActive() bool { return view.filePane.VisualActive() }

// Init implements userinterface.View.
func (view *View) Init() tea.Cmd { return view.load() }

// Resumed implements userinterface.Resumer. Nothing to do yet: the workspace refreshes
// only on R.
func (view *View) Resumed() tea.Cmd { return nil }

// ShortHelp implements userinterface.View.
func (view *View) ShortHelp() []key.Binding {
	return []key.Binding{view.bind.PaneLeft, view.bind.PaneRight, view.bind.FilePicker, view.km.Open}
}

// FullHelp implements userinterface.View. It also registers every multi-key sequence the
// workspace and its file pane react to.
func (view *View) FullHelp() [][]key.Binding {
	out := [][]key.Binding{
		{view.bind.PaneLeft, view.bind.PaneRight, view.bind.NextFile, view.bind.PreviousFile, view.bind.FilePicker},
		{view.bind.NextSection, view.bind.PreviousSection, view.bind.Comment, view.bind.Reply, view.bind.Viewed},
		{view.bind.Visual, view.bind.EditComment, view.bind.DeleteComment},
	}
	return append(out, view.filePane.FullHelp()...)
}

// Update implements userinterface.View.
func (view *View) Update(message tea.Msg) (userinterface.View, tea.Cmd) {
	if command, ok := view.review.update(message, view.reviewEnvironment()); ok {
		return view, command
	}
	if command, ok := view.merge.update(message, view.reviewEnvironment()); ok {
		return view, command
	}
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		view.width, view.height = message.Width, message.Height
		view.layout()
	case userinterface.ThemeMessage:
		view.th = message.Theme
		view.md = markdown.New(view.th)
		view.ov.invalidate()
		view.filePane.SetTheme(view.th)
		view.layout()
	case pullRequestLoadedMessage:
		if !view.mine(message.owner, message.repository, message.number, message.sequence) {
			return view, nil
		}
		command := view.pullRequestLoaded(message)
		if message.thenDiff {
			command = tea.Batch(command, view.fetchDiff(message))
		}
		return view, command
	case diffLoadedMessage:
		if !view.mine(message.owner, message.repository, message.number, message.sequence) {
			return view, nil
		}
		return view, view.diffLoaded(message)
	case threadsLoadedMessage:
		if !view.mine(message.owner, message.repository, message.number, message.sequence) {
			return view, nil
		}
		return view, view.threadsLoaded(message)
	case viewedMessage:
		if view.viewedMine(message) {
			return view, view.viewedDone(message)
		}
	case editedMessage:
		if !view.composeMine(origin{message.owner, message.repository, message.number}, message.sequence) {
			return view, nil
		}
		return view, view.edited(message)
	case postedMessage:
		// A post is never superseded by a later compose, so it is identified
		// by owner, repo and number only.
		if !view.owns(message.origin) {
			return view, nil
		}
		return view, view.posted(message)
	case commentDeletedMessage:
		if !view.owns(message.origin) {
			return view, nil
		}
		return view, view.commentDeleted(message)
	case commentCheckedMessage:
		if !view.owns(message.origin) {
			return view, nil
		}
		view.writeDone() // the write waits for the answer
		return view, userinterface.Confirm(message.question, message.onYes)
	case commentAgreedMessage:
		if !view.owns(message.origin) {
			return view, nil
		}
		return view, view.commentAgreed(message)
	case commentReadMessage:
		if !view.owns(message.origin) || message.sequence != view.cseq { // another editor started meanwhile
			return view, nil
		}
		return view, view.commentRead(message)
	case keys.Message:
		return view, view.handleKey(message)
	case tea.KeyPressMsg:
		if view.visualActive() {
			return view, view.visualKey(message)
		}
		if view.review.chooser != nil {
			return view, view.review.chooserKey(message, view.reviewEnvironment())
		}
		if view.picker != nil {
			view.pickerKey(message)
		}
	}
	return view, nil
}
