package ui

import (
	"io"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// Defaults for Options.
const (
	DefaultSeqTimeout = 500 * time.Millisecond
	DefaultMessageTTL = 4 * time.Second
)

// TickFunc schedules fn after d, like tea.Tick. Tests inject their own.
type TickFunc func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd

// Options configures the root Model. Zero values select defaults.
type Options struct {
	Logger     *slog.Logger
	SeqTimeout time.Duration // how long a pending key sequence waits
	MessageTTL time.Duration // how long info/error messages stay
	Tick       TickFunc
}

// Internal timer messages. The id/gen fields discard timers that were
// superseded before they fired.
type (
	seqTimeoutMsg  struct{ gen int }
	clearStatusMsg struct{ id int }
)

// Model is the root Bubble Tea model. It owns the view stack, key sequence
// resolution, the status bar, and the help overlay.
type Model struct {
	stack   []View
	th      theme.Theme
	keys    keys.Global
	log     *slog.Logger
	tick    TickFunc
	seqWait time.Duration
	msgTTL  time.Duration

	width, height int
	matcher       keys.Matcher
	seqGen        int

	showHelp bool
	cmdline  commandLine
	status   struct {
		text string
		kind statusKind
		id   int
	}
	spinning bool // the root spinner's tick chain is running
	spin     spinner.Model
}

// New returns a root model whose stack holds just root.
func New(root View, th theme.Theme, opts Options) *Model {
	if opts.SeqTimeout == 0 {
		opts.SeqTimeout = DefaultSeqTimeout
	}
	if opts.MessageTTL == 0 {
		opts.MessageTTL = DefaultMessageTTL
	}
	if opts.Tick == nil {
		opts.Tick = tea.Tick
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Model{
		stack:   []View{root},
		th:      th,
		keys:    keys.DefaultGlobal(),
		log:     opts.Logger,
		tick:    opts.Tick,
		seqWait: opts.SeqTimeout,
		msgTTL:  opts.MessageTTL,
		cmdline: newCommandLine(),
		spin:    spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(th.Spinner)),
	}
}

// Init starts the root view and asks the terminal for its background colour so
// the theme can match light and dark terminals.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.top().Init(), tea.RequestBackgroundColor)
}

func (m *Model) top() View { return m.stack[len(m.stack)-1] }

func (m *Model) contentSize() (w, h int) { return m.width, max(0, m.height-1) }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	return m, tea.Batch(cmd, m.syncSpinner())
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.cmdline.input.SetWidth(max(1, msg.Width-2)) // room for the prompt and cursor
		return m.broadcast(m.sizeMsg())
	case tea.BackgroundColorMsg:
		m.th = theme.New(msg.IsDark())
		m.spin.Style = m.th.Spinner
		return m.broadcast(ThemeMsg{Theme: m.th})
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case PushMsg:
		return m.push(msg.View)
	case PopMsg:
		return m.pop()
	case QuitMsg:
		return tea.Quit
	case InfoMsg:
		return m.setStatus(msg.Text, statusInfo)
	case ErrorMsg:
		m.log.Warn("error shown to user", "err", msg.Err)
		text := "error"
		if msg.Err != nil {
			text = msg.Err.Error()
		}
		return m.setStatus(text, statusError)
	case clearStatusMsg:
		if msg.id == m.status.id {
			m.status.text, m.status.kind = "", statusNone
		}
		return nil
	case seqTimeoutMsg:
		return m.expireSequence(msg.gen)
	case spinner.TickMsg:
		if msg.ID != m.spin.ID() {
			return m.broadcastOther(msg) // a view's own spinner
		}
		if !m.spinning {
			return nil // let the tick chain die while idle
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return cmd
	default:
		return m.broadcastOther(msg)
	}
}

func (m *Model) sizeMsg() tea.WindowSizeMsg {
	w, h := m.contentSize()
	return tea.WindowSizeMsg{Width: w, Height: h}
}

// broadcast sends msg to every view in the stack.
func (m *Model) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.stack))
	for i, v := range m.stack {
		m.stack[i], cmds[i] = v.Update(msg)
	}
	return tea.Batch(cmds...)
}

// broadcastOther sends a message the root does not consume to every view (and
// to the command line, for cursor blink).
func (m *Model) broadcastOther(msg tea.Msg) tea.Cmd {
	if m.cmdline.active {
		return tea.Batch(m.cmdline.update(msg), m.broadcast(msg))
	}
	return m.broadcast(msg)
}

func (m *Model) forward(msg tea.Msg) tea.Cmd {
	v, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = v
	return cmd
}

func (m *Model) push(v View) tea.Cmd {
	if v == nil {
		return nil
	}
	m.resetKeys()
	m.stack = append(m.stack, v)
	m.log.Debug("push view", "title", v.Title(), "depth", len(m.stack))
	return tea.Batch(v.Init(), m.forward(m.sizeMsg()))
}

func (m *Model) pop() tea.Cmd {
	m.resetKeys()
	if len(m.stack) == 1 {
		return tea.Quit
	}
	m.stack = m.stack[:len(m.stack)-1]
	m.log.Debug("pop view", "depth", len(m.stack))
	if r, ok := m.top().(Resumer); ok {
		return r.Resumed()
	}
	return nil
}

func (m *Model) resetKeys() {
	m.showHelp = false
	m.cmdline.close()
	m.matcher.Reset()
	m.seqGen++
}

func (m *Model) setStatus(text string, kind statusKind) tea.Cmd {
	m.status.id++
	m.status.text, m.status.kind = text, kind
	id := m.status.id
	return m.tick(m.msgTTL, func(time.Time) tea.Msg { return clearStatusMsg{id: id} })
}

// syncSpinner starts the root spinner's tick chain when the top view is
// loading and lets it die when it is not. It runs after every Update, so
// push, pop and view state changes are all covered.
func (m *Model) syncSpinner() tea.Cmd {
	l, ok := m.top().(Loader)
	want := ok && l.Loading()
	switch {
	case want && !m.spinning:
		m.spinning = true
		return m.spin.Tick
	case !want:
		m.spinning = false
	}
	return nil
}

// bindings returns every binding active now, for sequence matching.
func (m *Model) bindings() []key.Binding {
	out := m.keys.Bindings()
	for _, g := range m.top().FullHelp() {
		out = append(out, g...)
	}
	return out
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if m.cmdline.active {
		line, ok, cmd := m.cmdline.handleKey(msg)
		if ok {
			return parseCommand(line)
		}
		return cmd
	}
	if m.showHelp {
		if key.Matches(msg, m.keys.Help, m.keys.Cancel) {
			m.showHelp = false
		}
		return nil
	}
	if c, ok := m.top().(KeyCapturer); ok && c.CapturesKeys() {
		m.resetKeys()
		return m.forward(msg)
	}

	res := m.matcher.Feed(k, m.bindings())
	m.seqGen++
	switch res.Kind {
	case keys.Pending:
		gen := m.seqGen
		return m.tick(m.seqWait, func(time.Time) tea.Msg { return seqTimeoutMsg{gen: gen} })
	case keys.Matched:
		return m.dispatch(res.Seq)
	default:
		return m.forward(msg)
	}
}

func (m *Model) expireSequence(gen int) tea.Cmd {
	if gen != m.seqGen || !m.matcher.Pending() {
		return nil
	}
	m.seqGen++
	if res := m.matcher.Expire(m.bindings()); res.Kind == keys.Matched {
		return m.dispatch(res.Seq)
	}
	return nil
}

// dispatch acts on a resolved key or sequence: the root consumes Help and
// Command, everything else goes to the view.
func (m *Model) dispatch(seq string) tea.Cmd {
	km := keys.Msg{Seq: seq}
	switch {
	case key.Matches(km, m.keys.Help):
		m.showHelp = true
		return nil
	case key.Matches(km, m.keys.Command):
		return m.cmdline.open()
	}
	return m.forward(km)
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	var v tea.View
	if m.width > 0 && m.height > 0 {
		v = tea.NewView(m.content() + "\n" + m.bottomRow())
	}
	v.AltScreen = true
	return v
}

// content renders the area above the status bar, padded to full size.
func (m *Model) content() string {
	w, h := m.contentSize()
	var s string
	if m.showHelp {
		s = renderHelp(m.th, m.top().Title(), m.keys.FullHelp(), m.top().FullHelp(), w)
	} else {
		s = m.top().Render(w, h)
	}
	s = m.th.Normal.MaxWidth(w).MaxHeight(h).Render(s)
	if n := h - lipgloss.Height(s); n > 0 {
		s += strings.Repeat("\n", n)
	}
	return s
}

func (m *Model) bar() statusBar {
	crumbs := make([]string, len(m.stack))
	for i, v := range m.stack {
		crumbs[i] = v.Title()
	}
	b := statusBar{
		crumbs:  strings.Join(crumbs, " › "),
		text:    m.status.text,
		kind:    m.status.kind,
		pending: m.matcher.Display(),
	}
	if b.kind == statusNone {
		b.text = shortHelp(m.top().ShortHelp())
	}
	if m.spinning {
		b.spinner = m.spin.View()
	}
	return b
}

func shortHelp(bs []key.Binding) string {
	parts := []string{"? help"}
	for _, b := range bs {
		if h := b.Help(); b.Enabled() && h.Desc != "" {
			parts = append(parts, h.Key+" "+h.Desc)
		}
	}
	return strings.Join(parts, "  ")
}

// bottomRow is the status bar, or the command line while it is open.
func (m *Model) bottomRow() string {
	if m.cmdline.active {
		return m.th.StatusCommand.Width(m.width).MaxWidth(m.width).MaxHeight(1).Render(m.cmdline.input.View())
	}
	return m.bar().render(m.th, m.width)
}
