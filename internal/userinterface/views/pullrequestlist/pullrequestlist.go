// Package prlist is the screen that lists a repository's open pull requests.
package pullrequestlist

import (
	"context"
	"fmt"
	"io"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/scroll"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/views/workspace"
)

const (
	// loadMoreMargin is how close to the end of the loaded rows the selection
	// gets before the next page is requested.
	loadMoreMargin = 5
	requestTimeout = 30 * time.Second
)

// prsLoadedMsg is the result of a ListPullRequests call. repo and seq identify
// the request: every view receives every command result, and a refresh
// supersedes any page still in flight.
type pullRequestsLoadedMessage struct {
	repository github.Repository
	sequence   int
	// more is true for a next-page fetch that is appended to the list, false
	// for a first-page fetch that replaces it.
	more bool
	page github.Page[github.PullRequest]
	err  error
}

// Options configures New. Zero values select the real behaviour.
type Options struct {
	// Now supplies the current time for relative ages.
	Now func() time.Time
	// Open opens a URL in the browser; it runs inside a tea.Cmd.
	Open func(url string) error
}

// View lists one repository's open pull requests.
type View struct {
	client     github.Client
	repository github.Repository
	th         theme.Theme
	km         keys.Global
	now        func() time.Time
	open       func(url string) error

	pullRequests []github.PullRequest
	next         string // cursor of the next page, empty when all are loaded
	sequence     int    // latest request; older results are ignored
	loading      bool   // a first-page (re)load is in flight
	loadingMore  bool   // a next-page load is in flight
	err          error  // the last failed load, cleared by the next success
	moreFailed   bool   // the last next-page load failed; no auto-retry until R

	win           scroll.Window // cursor and top row over prs
	width, height int
}

var (
	_ userinterface.View   = (*View)(nil)
	_ userinterface.Loader = (*View)(nil)
)

// New returns the PR list for repo. It starts loading when pushed (Init).
func New(client github.Client, repo github.Repository, th theme.Theme, opts Options) *View {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Open == nil {
		opts.Open = openInBrowser
	}
	return &View{
		client:     client,
		repository: repo,
		th:         th,
		km:         keys.DefaultGlobal(),
		now:        opts.Now,
		open:       opts.Open,
		sequence:   1,
		loading:    true,
	}
}

// openInBrowser discards the launcher's output: it must not touch the screen
// the TUI owns. This covers a configured launcher; the default fallback writes
// to cli/browser's globals, which cmd/strig silences at startup.
func openInBrowser(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}

// Loading implements userinterface.Loader.
func (v *View) Loading() bool { return v.loading || v.loadingMore }

// Title implements userinterface.View.
func (v *View) Title() string { return v.repositoryName() }

// repoName is the repository name made safe to draw (it can come from -R).
func (v *View) repositoryName() string { return terminaltext.Clean(v.repository.String()) }

// ShortHelp implements userinterface.View.
func (v *View) ShortHelp() []key.Binding {
	return []key.Binding{v.km.Select, v.km.Open, v.km.Refresh}
}

// FullHelp implements userinterface.View. Every binding here is a global one the root
// already lists.
func (v *View) FullHelp() [][]key.Binding { return nil }

// Init implements userinterface.View.
func (v *View) Init() tea.Cmd { return v.fetch(github.PageRequest{}, false) }

// fetch loads one page. The closure captures copies, so it shares no state
// with the view.
func (v *View) fetch(req github.PageRequest, more bool) tea.Cmd {
	client, repo, seq := v.client, v.repository, v.sequence
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := client.ListPullRequests(ctx, repo, req)
		return pullRequestsLoadedMessage{repository: repo, sequence: seq, more: more, page: page, err: err}
	}
}

// Update implements userinterface.View.
func (v *View) Update(msg tea.Msg) (userinterface.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
		v.win.SetSize(v.listHeight())
		return v, nil
	case userinterface.ThemeMessage:
		v.th = msg.Theme
	case pullRequestsLoadedMessage:
		return v, v.loaded(msg)
	case keys.Message:
		return v, v.handleKey(msg)
	}
	return v, nil
}

func (v *View) loaded(msg pullRequestsLoadedMessage) tea.Cmd {
	if msg.repository != v.repository || msg.sequence != v.sequence {
		return nil // meant for another view, or superseded by a refresh
	}
	if msg.more {
		v.loadingMore = false
	} else {
		v.loading = false
	}
	if msg.err != nil {
		v.err = msg.err
		v.moreFailed = msg.more // stop auto-paging until R

		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("load pull requests in %s: %w", v.repository, msg.err)))
	}
	v.err = nil
	v.next = msg.page.Next
	if msg.more {
		v.pullRequests = append(v.pullRequests, msg.page.Items...)
	} else {
		v.replace(msg.page.Items)
	}
	v.win.SetRows(len(v.pullRequests))
	return v.maybeLoadMore()
}

// replace swaps in a fresh first page, keeping the selection on the same pull
// request when it is still listed.
func (v *View) replace(items []github.PullRequest) {
	var selected int
	if pr, ok := v.selected(); ok {
		selected = pr.Number
	}
	v.pullRequests = items
	// If the selected PR is gone, stay on the same row (the window clamps it).
	v.win.SetRows(len(items))
	for i, pr := range items {
		if selected != 0 && pr.Number == selected {
			v.win.Restore(i, v.win.Top(), 0)
			break
		}
	}
}

func (v *View) selected() (github.PullRequest, bool) {
	if v.win.Cursor() >= len(v.pullRequests) {
		return github.PullRequest{}, false
	}
	return v.pullRequests[v.win.Cursor()], true
}

func (v *View) handleKey(msg keys.Message) tea.Cmd {
	switch {
	case key.Matches(msg, v.km.Down):
		v.win.Down(1)
	case key.Matches(msg, v.km.Up):
		v.win.Up(1)
	case key.Matches(msg, v.km.Top):
		v.win.GotoTop()
	case key.Matches(msg, v.km.Bottom):
		v.win.GotoBottom()
	case key.Matches(msg, v.km.HalfDown):
		v.win.HalfPageDown()
	case key.Matches(msg, v.km.HalfUp):
		v.win.HalfPageUp()
	case key.Matches(msg, v.km.PageDown):
		v.win.PageDown()
	case key.Matches(msg, v.km.PageUp):
		v.win.PageUp()
	case key.Matches(msg, v.km.Select):
		return v.openSelected()
	case key.Matches(msg, v.km.Open):
		return v.browseSelected()
	case key.Matches(msg, v.km.Refresh):
		return v.refresh()
	default:
		return nil
	}
	return v.maybeLoadMore()
}

// refresh reloads from the first page. The old rows stay visible until the
// new ones arrive.
func (v *View) refresh() tea.Cmd {
	v.sequence++
	v.loading, v.loadingMore, v.moreFailed = true, false, false
	// Intentional limit: the API caps a page at 100 rows, so past row 100 the
	// refreshed list is shorter than the old one and the selection falls back
	// to the same row index, which may be a different PR. The rest pages in
	// lazily as before.
	// Ask for as many rows as are loaded (up to the API maximum) so the rows
	// the user has paged through survive; zero means the default size.
	return v.fetch(github.PageRequest{Limit: min(len(v.pullRequests), github.MaxPageSize)}, false)
}

// maybeLoadMore requests the next page when the selection is near the end of
// what is loaded.
func (v *View) maybeLoadMore() tea.Cmd {
	if v.next == "" || v.loading || v.moreFailed || v.loadingMore || v.win.Cursor() < len(v.pullRequests)-1-loadMoreMargin {
		return nil
	}
	v.loadingMore = true
	return v.fetch(github.PageRequest{After: v.next}, true)
}

// openSelected pushes the PR workspace for the selected pull request. It
// hands over the list's theme, clock and browser opener so the workspace looks
// and behaves like its parent (and tests stay deterministic).
func (v *View) openSelected() tea.Cmd {
	pr, ok := v.selected()
	if !ok {
		return nil
	}
	return userinterface.Push(workspace.New(v.client, v.repository, pr.Number,
		workspace.WithTheme(v.th),
		workspace.WithClock(v.now),
		workspace.WithOpener(v.open),
	))
}

func (v *View) browseSelected() tea.Cmd {
	pr, ok := v.selected()
	if !ok {
		return nil
	}
	open, url := v.open, pr.URL
	return func() tea.Msg {
		if url == "" {
			return userinterface.ErrorMessage{Err: fmt.Errorf("pull request #%d has no URL", pr.Number)}
		}
		if err := open(url); err != nil {
			return userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("open #%d in browser: %w", pr.Number, err))}
		}
		return userinterface.InfoMessage{Text: fmt.Sprintf("opened #%d in browser", pr.Number)}
	}
}

func (v *View) listHeight() int { return max(0, v.height-1) } // minus the header row
