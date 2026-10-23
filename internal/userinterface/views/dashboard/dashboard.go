// Package dashboard is the default screen outside a checkout: tabs of
// sections, each a list of pull requests found by a search query, across
// repositories.
package dashboard

import (
	"context"
	"fmt"
	"io"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/scroll"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/views/workspace"
)

const requestTimeout = 30 * time.Second

// sectionLoadedMessage is the result of one section's search. The tab,
// section and sequence identify the request: every view receives every
// command result, and a refresh supersedes a search still in flight.
type sectionLoadedMessage struct {
	tab, section, sequence int
	page                   github.Page[github.PullRequest]
	err                    error
}

// Options configures New. Zero values select the real behaviour.
type Options struct {
	// Now supplies the current time for relative ages.
	Now func() time.Time
	// Open opens a URL in the browser; it runs inside a tea.Cmd.
	Open func(url string) error
}

// section is one search and what it found.
type section struct {
	name     string
	query    string // as shown
	search   string // as sent: query plus the sort qualifier
	limit    int
	sequence int // latest request; older results are ignored

	pullRequests []github.PullRequest
	loading      bool
	err          error
}

// tab is a named group of sections with its own cursor.
type tab struct {
	name      string
	sections  []section
	requested bool // the sections were asked for at least once

	selected int // index among all rows of all sections
	window   scroll.Window
}

// View shows the tabs.
type View struct {
	client     github.Client
	theme      theme.Theme
	globalKeys keys.Global
	bindings   bindings
	now        func() time.Time
	open       func(url string) error

	tabs          []tab
	active        int
	width, height int
}

type bindings struct {
	nextTab, previousTab         key.Binding
	nextSection, previousSection key.Binding
}

var (
	_ userinterface.View   = (*View)(nil)
	_ userinterface.Loader = (*View)(nil)
)

// New returns a dashboard of tabs. It loads the first tab when pushed (Init).
func New(client github.Client, tabs []config.Tab, activeTheme theme.Theme, options Options) *View {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Open == nil {
		options.Open = openInBrowser
	}
	view := &View{
		client:     client,
		theme:      activeTheme,
		globalKeys: keys.DefaultGlobal(),
		bindings: bindings{
			nextTab:         key.NewBinding(key.WithKeys("g t"), key.WithHelp("gt", "next tab")),
			previousTab:     key.NewBinding(key.WithKeys("g T"), key.WithHelp("gT", "previous tab")),
			nextSection:     key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next section")),
			previousSection: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous section")),
		},
		now:  options.Now,
		open: options.Open,
	}
	for _, configured := range tabs {
		current := tab{name: configured.Name}
		for _, configuredSection := range configured.Sections {
			current.sections = append(current.sections, section{
				name:   configuredSection.Name,
				query:  configuredSection.Query,
				search: searchQuery(configuredSection),
				limit:  configuredSection.Limit,
			})
		}
		view.tabs = append(view.tabs, current)
	}
	return view
}

// searchQuery adds the section's sort to its query. The config rejects sort:
// in queries, so this never duplicates one.
func searchQuery(configured config.Section) string {
	return configured.Query + " sort:" + string(configured.Sort) + "-" + string(configured.Order)
}

// openInBrowser discards the launcher's output: it must not touch the screen
// the TUI owns.
func openInBrowser(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}

// Loading implements userinterface.Loader.
func (view *View) Loading() bool {
	if len(view.tabs) == 0 {
		return false
	}
	for _, current := range view.tabs[view.active].sections {
		if current.loading {
			return true
		}
	}
	return false
}

// Title implements userinterface.View.
func (view *View) Title() string { return "Dashboard" }

// ShortHelp implements userinterface.View.
func (view *View) ShortHelp() []key.Binding {
	return []key.Binding{view.globalKeys.Select, view.globalKeys.Open, view.globalKeys.Refresh, view.bindings.nextTab}
}

// FullHelp implements userinterface.View. The movement and selection keys are
// global ones the root already lists.
func (view *View) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{view.bindings.nextTab, view.bindings.previousTab, view.bindings.nextSection, view.bindings.previousSection},
	}
}

// Init implements userinterface.View.
func (view *View) Init() tea.Cmd { return view.show(view.active) }

// show loads the tab's sections the first time it is shown.
func (view *View) show(index int) tea.Cmd {
	if index >= len(view.tabs) || view.tabs[index].requested {
		return nil
	}
	return view.load(index)
}

// load (re)requests every section of a tab. Rows already shown stay until the
// new ones arrive.
func (view *View) load(index int) tea.Cmd {
	current := &view.tabs[index]
	current.requested = true
	commands := make([]tea.Cmd, len(current.sections))
	for sectionIndex := range current.sections {
		target := &current.sections[sectionIndex]
		target.sequence++
		target.loading = true
		commands[sectionIndex] = view.search(index, sectionIndex, *target)
	}
	return tea.Batch(commands...)
}

// search runs one query. The closure captures copies, so it shares no state
// with the view.
func (view *View) search(tabIndex, sectionIndex int, target section) tea.Cmd {
	client := view.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := client.SearchPullRequests(ctx, target.search, github.PageRequest{Limit: target.limit})
		return sectionLoadedMessage{tab: tabIndex, section: sectionIndex, sequence: target.sequence, page: page, err: err}
	}
}

// Update implements userinterface.View.
func (view *View) Update(message tea.Msg) (userinterface.View, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		view.width, view.height = message.Width, message.Height
	case userinterface.ThemeMessage:
		view.theme = message.Theme
	case sectionLoadedMessage:
		view.loaded(message)
	case keys.Message:
		return view, view.handleKey(message)
	}
	return view, nil
}

func (view *View) loaded(message sectionLoadedMessage) {
	if message.tab >= len(view.tabs) || message.section >= len(view.tabs[message.tab].sections) {
		return
	}
	current := &view.tabs[message.tab]
	target := &current.sections[message.section]
	if message.sequence != target.sequence {
		return // superseded by a refresh
	}
	target.loading = false
	target.err = message.err
	if message.err != nil {
		return // keep the rows from the last good load
	}
	selected, hadSelection := current.selectedPullRequest()
	target.pullRequests = message.page.Items
	current.restore(selected, hadSelection)
}

func (view *View) handleKey(message keys.Message) tea.Cmd {
	if len(view.tabs) == 0 {
		return nil
	}
	current := &view.tabs[view.active]
	switch {
	case key.Matches(message, view.bindings.nextTab):
		return view.switchTab(1)
	case key.Matches(message, view.bindings.previousTab):
		return view.switchTab(-1)
	case key.Matches(message, view.bindings.nextSection):
		current.moveSection(1)
	case key.Matches(message, view.bindings.previousSection):
		current.moveSection(-1)
	case key.Matches(message, view.globalKeys.Down):
		current.move(1)
	case key.Matches(message, view.globalKeys.Up):
		current.move(-1)
	case key.Matches(message, view.globalKeys.Top):
		current.selected = 0
	case key.Matches(message, view.globalKeys.Bottom):
		current.selected = current.total() - 1
	case key.Matches(message, view.globalKeys.HalfDown):
		current.move(max(view.listHeight()/2, 1))
	case key.Matches(message, view.globalKeys.HalfUp):
		current.move(-max(view.listHeight()/2, 1))
	case key.Matches(message, view.globalKeys.PageDown):
		current.move(max(view.listHeight(), 1))
	case key.Matches(message, view.globalKeys.PageUp):
		current.move(-max(view.listHeight(), 1))
	case key.Matches(message, view.globalKeys.Select):
		return view.openSelected()
	case key.Matches(message, view.globalKeys.Open):
		return view.browseSelected()
	case key.Matches(message, view.globalKeys.Refresh):
		return view.load(view.active)
	default:
		return nil
	}
	current.clampSelection()
	return nil
}

// switchTab moves to the next (or previous) tab, wrapping around.
func (view *View) switchTab(step int) tea.Cmd {
	view.active = (view.active + step + len(view.tabs)) % len(view.tabs)
	return view.show(view.active)
}

// listHeight is the rows left for sections under the tab bar and column header.
func (view *View) listHeight() int { return max(0, view.height-2) }

func (view *View) selected() (github.PullRequest, bool) {
	if len(view.tabs) == 0 {
		return github.PullRequest{}, false
	}
	return view.tabs[view.active].selectedPullRequest()
}

// openSelected pushes the PR workspace for the selected pull request, which
// can belong to any repository.
func (view *View) openSelected() tea.Cmd {
	pullRequest, ok := view.selected()
	if !ok {
		return nil
	}
	return userinterface.Push(workspace.New(view.client, pullRequest.Repository, pullRequest.Number,
		workspace.WithTheme(view.theme),
		workspace.WithClock(view.now),
		workspace.WithOpener(view.open),
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
			return userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("pull request %s#%d has no URL", pullRequest.Repository, pullRequest.Number))}
		}
		if err := open(url); err != nil {
			return userinterface.ErrorMessage{Err: terminaltext.SafeError(fmt.Errorf("open %s#%d in browser: %w", pullRequest.Repository, pullRequest.Number, err))}
		}
		return userinterface.InfoMessage{Text: fmt.Sprintf("opened %s#%d in browser", terminaltext.Clean(pullRequest.Repository.String()), pullRequest.Number)}
	}
}
