// Package workspace is the PR workspace: one screen per pull request made of
// tiled panes. The left Files pane lists an Overview entry and the changed
// files; the right pane shows the PR description and conversation, or the
// selected file's diff.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/components/markdown"
	"github.com/MikePors/strigidae/internal/ui/components/picker"
	"github.com/MikePors/strigidae/internal/ui/components/scroll"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/termtext"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// pane identifies which pane has focus.
type pane int

const (
	paneFiles pane = iota
	paneRight
)

// Result messages carry the identity of the request that produced them
// (repo, number and the view's request sequence), because every view in the
// stack receives every non-key message.
type (
	prLoadedMsg struct {
		repo   github.Repo
		number int
		seq    int
		detail github.PullRequestDetail
		err    error
	}
	diffLoadedMsg struct {
		repo   github.Repo
		number int
		seq    int
		raw    string
		parsed *diff.Diff // parsed in the command, so Update stays cheap
		err    error
	}
)

// bindings are the keys specific to the workspace. Movement keys come from
// keys.Global.
type bindings struct {
	PaneLeft, PaneRight      key.Binding
	NextFile, PrevFile       key.Binding
	FilePicker               key.Binding
	NextSection, PrevSection key.Binding
}

func defaultBindings() bindings {
	b := func(k, helpKey, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(helpKey, desc))
	}
	return bindings{
		PaneLeft:    b("h", "h", "focus files pane"),
		PaneRight:   b("l", "l", "focus right pane"),
		NextFile:    b("] f", "]f", "next file"),
		PrevFile:    b("[ f", "[f", "previous file"),
		FilePicker:  b("f", "f", "file picker"),
		NextSection: b("] s", "]s", "next section (overview)"),
		PrevSection: b("[ s", "[s", "previous section (overview)"),
	}
}

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

	seq         int // latest load request; older results are stale
	prPending   bool
	diffPending bool
	detail      *github.PullRequestDetail
	diff        DiffState
	err         error // load failure while there is nothing to show
	notFound    bool

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
	}
	for _, o := range opts {
		o(v)
	}
	v.md = markdown.New(v.th.IsDark)
	v.ov = newOverview()
	if v.filePane == nil {
		v.filePane = newDiffPane(v.th)
	}
	v.filePane.SetTheme(v.th)
	return v
}

func openInBrowser(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}

// safeError is an error whose text is safe to draw: API errors can carry
// attacker-controlled text, so it is cleaned when displayed.
type safeError struct{ err error }

func (e safeError) Error() string { return termtext.Clean(e.err.Error()) }
func (e safeError) Unwrap() error { return e.err }

// Title implements ui.View.
func (v *View) Title() string { return fmt.Sprintf("#%d", v.number) }

// Loading implements ui.Loader.
func (v *View) Loading() bool { return v.prPending || v.diffPending }

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
		{v.bind.NextSection, v.bind.PrevSection},
	}
	return append(out, v.filePane.FullHelp()...)
}

// load starts a fresh load of the pull request and its diff.
func (v *View) load() tea.Cmd {
	v.seq++
	v.prPending, v.diffPending = true, true
	v.diff.Loading = true
	v.filePane.SetDiff(v.diff)
	client, repo, number, seq := v.client, v.repo, v.number, v.seq
	return tea.Batch(
		func() tea.Msg {
			d, err := client.GetPullRequest(context.Background(), repo, number)
			return prLoadedMsg{repo: repo, number: number, seq: seq, detail: d, err: err}
		},
		func() tea.Msg {
			raw, err := client.GetDiff(context.Background(), repo, number)
			if err != nil {
				return diffLoadedMsg{repo: repo, number: number, seq: seq, err: err}
			}
			parsed, err := diff.Parse(raw)
			if err != nil {
				err = fmt.Errorf("parse diff of %s#%d: %w", repo, number, err)
			}
			return diffLoadedMsg{repo: repo, number: number, seq: seq, raw: raw, parsed: parsed, err: err}
		},
	)
}

func (v *View) mine(repo github.Repo, number, seq int) bool {
	return repo == v.repo && number == v.number && seq == v.seq
}

// Update implements ui.View.
func (v *View) Update(msg tea.Msg) (ui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
		v.layout()
	case ui.ThemeMsg:
		v.th = msg.Theme
		v.md = markdown.New(v.th.IsDark)
		v.ov.invalidate()
		v.filePane.SetTheme(v.th)
		v.layout()
	case prLoadedMsg:
		if !v.mine(msg.repo, msg.number, msg.seq) {
			return v, nil
		}
		return v, v.prLoaded(msg)
	case diffLoadedMsg:
		if !v.mine(msg.repo, msg.number, msg.seq) {
			return v, nil
		}
		return v, v.diffLoaded(msg)
	case keys.Msg:
		return v, v.handleKey(msg)
	case tea.KeyPressMsg:
		if v.picker != nil {
			v.pickerKey(msg)
		}
	}
	return v, nil
}

func (v *View) prLoaded(msg prLoadedMsg) tea.Cmd {
	v.prPending = false
	if msg.err != nil {
		if v.detail == nil {
			v.err, v.notFound = msg.err, errors.Is(msg.err, github.ErrNotFound)
		}
		return ui.Error(safeError{msg.err})
	}
	prev := v.selectedPath()
	v.detail, v.err, v.notFound = &msg.detail, nil, false
	v.fw.SetRows(v.entries())
	v.fw.GotoTop()
	if prev != "" {
		for i, f := range msg.detail.Files {
			if f.Path == prev {
				v.setSel(i + 1)
			}
		}
	}
	v.ov.invalidate()
	v.selectionChanged()
	v.layout()
	return nil
}

func (v *View) diffLoaded(msg diffLoadedMsg) tea.Cmd {
	v.diffPending = false
	if msg.err != nil && v.diff.Parsed != nil && !errors.Is(msg.err, github.ErrDiffTooLarge) {
		v.diff.Loading = false // keep the loaded diff; the error goes to the status bar
	} else {
		v.diff = DiffState{Raw: msg.raw, Parsed: msg.parsed, Err: msg.err}
	}
	v.filePane.SetDiff(v.diff)
	// A diff that is merely too large is shown in the diff pane, not as a
	// transient error.
	if msg.err != nil && !errors.Is(msg.err, github.ErrDiffTooLarge) {
		return ui.Error(safeError{msg.err})
	}
	return nil
}

// files returns the changed files, or nil before the pull request loads.
func (v *View) files() []github.ChangedFile {
	if v.detail == nil {
		return nil
	}
	return v.detail.Files
}

// entries is the number of rows in the Files pane: Overview plus the files.
func (v *View) entries() int { return 1 + len(v.files()) }

// sel is the selected Files row: 0 is Overview, i is Files[i-1].
func (v *View) sel() int { return v.fw.Cursor() }

// setSel moves the selection to row i, scrolling the Files list to show it.
func (v *View) setSel(i int) { v.fw.Restore(i, v.fw.Top(), 0) }

func (v *View) selectedPath() string {
	if s := v.sel(); s > 0 && s <= len(v.files()) {
		return v.files()[s-1].Path
	}
	return ""
}

func (v *View) selectionChanged() {
	if s := v.sel(); s > 0 && s <= len(v.files()) {
		f := v.files()[s-1]
		if f != v.paneFile {
			v.paneFile = f
			v.filePane.SetFile(f)
		}
	}
}

func (v *View) selectEntry(i int) {
	prev := v.sel()
	v.setSel(i)
	if v.sel() != prev {
		v.selectionChanged()
	}
	v.layout()
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
	case key.Matches(msg, v.bind.NextSection):
		v.jumpSection(1)
	case key.Matches(msg, v.bind.PrevSection):
		v.jumpSection(-1)
	case key.Matches(msg, v.km.Open):
		return v.openBrowser()
	case key.Matches(msg, v.km.Refresh):
		return v.load()
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
			return ui.ErrorMsg{Err: safeError{fmt.Errorf("open %s in browser: %w", termtext.Clean(url), err)}}
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

func (v *View) jumpSection(delta int) {
	if v.sel() != 0 || v.detail == nil {
		return
	}
	v.ov.jump(delta)
}

func (v *View) openPicker() {
	files := v.files()
	if len(files) == 0 {
		return
	}
	items := make([]string, len(files))
	paths := make([]string, len(files))
	for i, f := range files {
		items[i] = termtext.Clean(f.Path)
		paths[i] = f.Path
	}
	v.pickerItems, v.pickerPaths = items, paths
	v.picker = picker.New(v.th, "Go to file", items)
}

// pickerKey feeds a raw key to the open picker.
func (v *View) pickerKey(msg tea.KeyPressMsg) {
	res := v.picker.Update(msg)
	switch res.Outcome {
	case picker.Cancelled:
		v.picker = nil
	case picker.Chosen:
		v.picker = nil
		path := v.pickerPaths[res.Index]
		for i, f := range v.files() {
			if f.Path == path {
				v.selectEntry(i + 1)
			}
		}
	case picker.Open:
	}
}
