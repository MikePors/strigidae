// Package pullrequestlist is the screen that lists a repository's open pull requests.
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
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
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

// pullRequestsLoadedMessage is the result of a ListPullRequests call. repo and seq identify
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
	// Wrap makes the diffs of the pull requests it opens wrap long lines.
	Wrap bool
}

// View lists one repository's open pull requests.
type View struct {
	client     github.Client
	repository github.Repository
	theme      theme.Theme
	globalKeys keys.Global
	now        func() time.Time
	open       func(url string) error
	// hunkMarks keeps the hunks marked reviewed in every workspace opened
	// from here, so they survive closing and reopening a pull request.
	hunkMarks *workspace.HunkMarks
	wrap      bool // passed to every workspace opened from here

	pullRequests []github.PullRequest
	next         string // cursor of the next page, empty when all are loaded
	sequence     int    // latest request; older results are ignored
	loading      bool   // a first-page (re)load is in flight
	loadingMore  bool   // a next-page load is in flight
	err          error  // the last failed load, cleared by the next success
	moreFailed   bool   // the last next-page load failed; no auto-retry until R

	window        scroll.Window // cursor and top row over prs
	search        search.State
	width, height int
}

var (
	_ userinterface.View     = (*View)(nil)
	_ userinterface.Loader   = (*View)(nil)
	_ userinterface.Prompter = (*View)(nil)
)

// New returns the PR list for repository. It starts loading when pushed (Init).
func New(client github.Client, repository github.Repository, activeTheme theme.Theme, options Options) *View {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Open == nil {
		options.Open = openInBrowser
	}
	return &View{
		client:     client,
		repository: repository,
		theme:      activeTheme,
		globalKeys: keys.DefaultGlobal(),
		now:        options.Now,
		open:       options.Open,
		sequence:   1,
		loading:    true,
		hunkMarks:  workspace.NewHunkMarks(),
		wrap:       options.Wrap,
	}
}

// openInBrowser discards the launcher's output: it must not touch the screen
// the TUI owns. This covers a configured launcher; the default fallback writes
// to cli/browser's globals, which cmd/strig silences at startup.
func openInBrowser(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}

// Loading implements userinterface.Loader.
func (view *View) Loading() bool { return view.loading || view.loadingMore }

// Title implements userinterface.View.
func (view *View) Title() string { return view.repositoryName() }

// repositoryName is the repository name made safe to draw (it can come from -R).
func (view *View) repositoryName() string { return terminaltext.Clean(view.repository.String()) }

// ShortHelp implements userinterface.View.
func (view *View) ShortHelp() []key.Binding {
	return []key.Binding{view.globalKeys.Select, view.globalKeys.Open, view.globalKeys.Refresh}
}

// FullHelp implements userinterface.View. The search keys are global ones that
// only views with a list to search list.
func (view *View) FullHelp() [][]key.Binding {
	return [][]key.Binding{{view.globalKeys.Search, view.globalKeys.NextMatch, view.globalKeys.PreviousMatch}}
}

// Init implements userinterface.View.
func (view *View) Init() tea.Cmd { return view.fetch(github.PageRequest{}, false) }

// fetch loads one page. The closure captures copies, so it shares no state
// with the view.
func (view *View) fetch(pageRequest github.PageRequest, more bool) tea.Cmd {
	client, repository, sequence := view.client, view.repository, view.sequence
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := client.ListPullRequests(ctx, repository, pageRequest)
		return pullRequestsLoadedMessage{repository: repository, sequence: sequence, more: more, page: page, err: err}
	}
}

// Update implements userinterface.View.
func (view *View) Update(message tea.Msg) (userinterface.View, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		view.width, view.height = message.Width, message.Height
		view.window.SetSize(view.listHeight())
		return view, nil
	case userinterface.ThemeMessage:
		view.theme = message.Theme
	case pullRequestsLoadedMessage:
		return view, view.loaded(message)
	case userinterface.PromptResultMessage:
		return view, view.search.Prompt(message, view.searchList())
	case keys.Message:
		return view, view.handleKey(message)
	}
	return view, nil
}

func (view *View) loaded(message pullRequestsLoadedMessage) tea.Cmd {
	if message.repository != view.repository || message.sequence != view.sequence {
		return nil // meant for another view, or superseded by a refresh
	}
	if message.more {
		view.loadingMore = false
	} else {
		view.loading = false
	}
	if message.err != nil {
		view.err = message.err
		view.moreFailed = message.more // stop auto-paging until R

		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("load pull requests in %s: %w", view.repository, message.err)))
	}
	view.err = nil
	view.next = message.page.Next
	if message.more {
		view.pullRequests = append(view.pullRequests, message.page.Items...)
	} else {
		view.replace(message.page.Items)
	}
	view.window.SetRows(len(view.pullRequests))
	view.search.Refresh(view.searchList())
	return view.maybeLoadMore()
}

// replace swaps in a fresh first page, keeping the selection on the same pull
// request when it is still listed.
func (view *View) replace(items []github.PullRequest) {
	var selected int
	if pullRequest, ok := view.selected(); ok {
		selected = pullRequest.Number
	}
	view.pullRequests = items
	// If the selected PR is gone, stay on the same row (the window clamps it).
	view.window.SetRows(len(items))
	for i, pullRequest := range items {
		if selected != 0 && pullRequest.Number == selected {
			view.window.Restore(i, view.window.Top(), 0)
			break
		}
	}
}

func (view *View) selected() (github.PullRequest, bool) {
	if view.window.Cursor() >= len(view.pullRequests) {
		return github.PullRequest{}, false
	}
	return view.pullRequests[view.window.Cursor()], true
}

func (view *View) handleKey(message keys.Message) tea.Cmd {
	switch {
	case key.Matches(message, view.globalKeys.Down):
		view.window.Down(message.Repeat())
	case key.Matches(message, view.globalKeys.Up):
		view.window.Up(message.Repeat())
	case key.Matches(message, view.globalKeys.Top):
		view.window.GotoTop()
	case key.Matches(message, view.globalKeys.Bottom):
		if message.Count > 0 {
			view.window.GotoRow(message.Count - 1)
		} else {
			view.window.GotoBottom()
		}
	case key.Matches(message, view.globalKeys.HalfDown):
		message.Each(view.window.Cursor, view.window.HalfPageDown)
	case key.Matches(message, view.globalKeys.HalfUp):
		message.Each(view.window.Cursor, view.window.HalfPageUp)
	case key.Matches(message, view.globalKeys.PageDown):
		message.Each(view.window.Cursor, view.window.PageDown)
	case key.Matches(message, view.globalKeys.PageUp):
		message.Each(view.window.Cursor, view.window.PageUp)
	case key.Matches(message, view.globalKeys.ScrollCursorTop):
		view.window.ScrollCursorTop()
	case key.Matches(message, view.globalKeys.ScrollCursorMiddle):
		view.window.ScrollCursorMiddle()
	case key.Matches(message, view.globalKeys.ScrollCursorBottom):
		view.window.ScrollCursorBottom()
	case key.Matches(message, view.globalKeys.Search):
		return view.search.Start(view.searchList())
	case key.Matches(message, view.globalKeys.NextMatch):
		return view.search.Step(message, true, view.searchList())
	case key.Matches(message, view.globalKeys.PreviousMatch):
		return view.search.Step(message, false, view.searchList())
	case key.Matches(message, view.globalKeys.Cancel):
		view.search.Clear()
	case key.Matches(message, view.globalKeys.Select):
		return view.openSelected()
	case key.Matches(message, view.globalKeys.Open):
		return view.browseSelected()
	case key.Matches(message, view.globalKeys.Refresh):
		return view.refresh()
	default:
		return nil
	}
	return view.maybeLoadMore()
}

// refresh reloads from the first page. The old rows stay visible until the
// new ones arrive.
func (view *View) refresh() tea.Cmd {
	view.sequence++
	view.loading, view.loadingMore, view.moreFailed = true, false, false
	// Intentional limit: the API caps a page at 100 rows, so past row 100 the
	// refreshed list is shorter than the old one and the selection falls back
	// to the same row index, which may be a different PR. The rest pages in
	// lazily as before.
	// Ask for as many rows as are loaded (up to the API maximum) so the rows
	// the user has paged through survive; zero means the default size.
	return view.fetch(github.PageRequest{Limit: min(len(view.pullRequests), github.MaximumPageSize)}, false)
}

// maybeLoadMore requests the next page when the selection is near the end of
// what is loaded.
func (view *View) maybeLoadMore() tea.Cmd {
	if view.next == "" || view.loading || view.moreFailed || view.loadingMore || view.window.Cursor() < len(view.pullRequests)-1-loadMoreMargin {
		return nil
	}
	view.loadingMore = true
	return view.fetch(github.PageRequest{After: view.next}, true)
}

// openSelected pushes the PR workspace for the selected pull request. It
// hands over the list's theme, clock and browser opener so the workspace looks
// and behaves like its parent (and tests stay deterministic).
func (view *View) openSelected() tea.Cmd {
	pullRequest, ok := view.selected()
	if !ok {
		return nil
	}
	return userinterface.Push(workspace.New(view.client, view.repository, pullRequest.Number,
		workspace.WithTheme(view.theme),
		workspace.WithClock(view.now),
		workspace.WithOpener(view.open),
		workspace.WithHunkMarks(view.hunkMarks),
		workspace.WithWrap(view.wrap),
	))
}

func (view *View) browseSelected() tea.Cmd {
	pullRequest, ok := view.selected()
	if !ok {
		return nil
	}
	open, url := view.open, pullRequest.URL
	return func() tea.Msg {
		if url == "" {
			return userinterface.ErrorMessage{Err: fmt.Errorf("pull request #%d has no URL", pullRequest.Number)}
		}
		if err := open(url); err != nil {
			return userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("open #%d in browser: %w", pullRequest.Number, err))}
		}
		return userinterface.InfoMessage{Text: fmt.Sprintf("opened #%d in browser", pullRequest.Number)}
	}
}

func (view *View) listHeight() int { return max(0, view.height-1) } // minus the header row
