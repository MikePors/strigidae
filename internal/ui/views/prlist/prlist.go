// Package prlist is the screen that lists a repository's open pull requests.
package prlist

import (
	"context"
	"fmt"
	"io"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
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
type prsLoadedMsg struct {
	repo github.Repo
	seq  int
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
	client github.Client
	repo   github.Repo
	th     theme.Theme
	km     keys.Global
	now    func() time.Time
	open   func(url string) error

	prs         []github.PullRequest
	next        string // cursor of the next page, empty when all are loaded
	seq         int    // latest request; older results are ignored
	loading     bool   // a first-page (re)load is in flight
	loadingMore bool   // a next-page load is in flight
	err         error  // the last failed load, cleared by the next success
	moreFailed  bool   // the last next-page load failed; no auto-retry until R

	cursor, top   int
	width, height int
}

var (
	_ ui.View   = (*View)(nil)
	_ ui.Loader = (*View)(nil)
)

// New returns the PR list for repo. It starts loading when pushed (Init).
func New(client github.Client, repo github.Repo, th theme.Theme, opts Options) *View {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Open == nil {
		opts.Open = openInBrowser
	}
	return &View{
		client:  client,
		repo:    repo,
		th:      th,
		km:      keys.DefaultGlobal(),
		now:     opts.Now,
		open:    opts.Open,
		seq:     1,
		loading: true,
	}
}

// openInBrowser discards the launcher's output: it must not touch the screen
// the TUI owns. This covers a configured launcher; the default fallback writes
// to cli/browser's globals, which cmd/strig silences at startup.
func openInBrowser(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}

// Loading implements ui.Loader.
func (v *View) Loading() bool { return v.loading || v.loadingMore }

// Title implements ui.View.
func (v *View) Title() string { return v.repo.String() }

// ShortHelp implements ui.View.
func (v *View) ShortHelp() []key.Binding {
	return []key.Binding{v.km.Select, v.km.Open, v.km.Refresh}
}

// FullHelp implements ui.View. Every binding here is a global one the root
// already lists.
func (v *View) FullHelp() [][]key.Binding { return nil }

// Init implements ui.View.
func (v *View) Init() tea.Cmd { return v.fetch(github.PageRequest{}, false) }

// fetch loads one page. The closure captures copies, so it shares no state
// with the view.
func (v *View) fetch(req github.PageRequest, more bool) tea.Cmd {
	client, repo, seq := v.client, v.repo, v.seq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := client.ListPullRequests(ctx, repo, req)
		return prsLoadedMsg{repo: repo, seq: seq, more: more, page: page, err: err}
	}
}

// Update implements ui.View.
func (v *View) Update(msg tea.Msg) (ui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
		v.clamp()
		return v, nil
	case ui.ThemeMsg:
		v.th = msg.Theme
	case prsLoadedMsg:
		return v, v.loaded(msg)
	case keys.Msg:
		return v, v.handleKey(msg)
	}
	return v, nil
}

func (v *View) loaded(msg prsLoadedMsg) tea.Cmd {
	if msg.repo != v.repo || msg.seq != v.seq {
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

		return ui.Error(fmt.Errorf("load pull requests in %s: %w", v.repo, msg.err))
	}
	v.err = nil
	v.next = msg.page.Next
	if msg.more {
		v.prs = append(v.prs, msg.page.Items...)
	} else {
		v.replace(msg.page.Items)
	}
	v.clamp()
	return v.maybeLoadMore()
}

// replace swaps in a fresh first page, keeping the selection on the same pull
// request when it is still listed.
func (v *View) replace(items []github.PullRequest) {
	var selected int
	if pr, ok := v.selected(); ok {
		selected = pr.Number
	}
	v.prs = items
	v.cursor = 0
	for i, pr := range items {
		if selected != 0 && pr.Number == selected {
			v.cursor = i
			break
		}
	}
}

func (v *View) selected() (github.PullRequest, bool) {
	if v.cursor < 0 || v.cursor >= len(v.prs) {
		return github.PullRequest{}, false
	}
	return v.prs[v.cursor], true
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
		v.cursor = len(v.prs) - 1
	case key.Matches(msg, v.km.HalfDown):
		v.cursor += page / 2
	case key.Matches(msg, v.km.HalfUp):
		v.cursor -= page / 2
	case key.Matches(msg, v.km.PageDown):
		v.cursor += page
	case key.Matches(msg, v.km.PageUp):
		v.cursor -= page
	case key.Matches(msg, v.km.Select):
		return v.openSelected()
	case key.Matches(msg, v.km.Open):
		return v.browseSelected()
	case key.Matches(msg, v.km.Refresh):
		return v.refresh()
	default:
		return nil
	}
	v.clamp()
	return v.maybeLoadMore()
}

// refresh reloads from the first page. The old rows stay visible until the
// new ones arrive.
func (v *View) refresh() tea.Cmd {
	v.seq++
	v.loading, v.loadingMore, v.moreFailed = true, false, false
	return v.fetch(github.PageRequest{}, false)
}

// maybeLoadMore requests the next page when the selection is near the end of
// what is loaded.
func (v *View) maybeLoadMore() tea.Cmd {
	if v.next == "" || v.loading || v.moreFailed || v.loadingMore || v.cursor < len(v.prs)-1-loadMoreMargin {
		return nil
	}
	v.loadingMore = true
	return v.fetch(github.PageRequest{After: v.next}, true)
}

// openSelected is the hook for opening the PR workspace. Once that view
// exists, replace the body with:
//
//	return ui.Push(workspace.New(v.client, pr, v.th))
func (v *View) openSelected() tea.Cmd {
	if _, ok := v.selected(); !ok {
		return nil
	}
	return ui.Info("PR workspace arrives with #7")
}

func (v *View) browseSelected() tea.Cmd {
	pr, ok := v.selected()
	if !ok {
		return nil
	}
	open, url := v.open, pr.URL
	return func() tea.Msg {
		if url == "" {
			return ui.ErrorMsg{Err: fmt.Errorf("pull request #%d has no URL", pr.Number)}
		}
		if err := open(url); err != nil {
			return ui.ErrorMsg{Err: fmt.Errorf("open #%d in browser: %w", pr.Number, err)}
		}
		return ui.InfoMsg{Text: fmt.Sprintf("opened #%d in browser", pr.Number)}
	}
}

func (v *View) listHeight() int { return max(0, v.height-1) } // minus the header row

// clamp keeps the cursor in range and the window scrolled to show it.
func (v *View) clamp() {
	v.cursor = min(max(v.cursor, 0), max(0, len(v.prs)-1))
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
	v.top = min(max(v.top, 0), max(0, len(v.prs)-h))
}
