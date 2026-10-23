package userinterface

import (
	"io"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// Defaults for Options.
const (
	DefaultSequenceTimeout = 500 * time.Millisecond
	DefaultMessageLifetime = 4 * time.Second
)

// TickFunc schedules fn after d, like tea.Tick. Tests inject their own.
type TickFunction func(duration time.Duration, function func(time.Time) tea.Msg) tea.Cmd

// Options configures the root Model. Zero values select defaults.
type Options struct {
	Logger          *slog.Logger
	SequenceTimeout time.Duration // how long a pending key sequence waits
	MessageLifetime time.Duration // how long info/error messages stay
	Tick            TickFunction
}

// Internal timer messages. The id/gen fields discard timers that were
// superseded before they fired.
type (
	sequenceTimeoutMessage struct{ generation int }
	clearStatusMessage     struct{ identifier int }
)

// Model is the root Bubble Tea model. It owns the view stack, key sequence
// resolution, the status bar, and the help overlay.
type Model struct {
	stack           []View
	th              theme.Theme
	keys            keys.Global
	log             *slog.Logger
	tick            TickFunction
	sequenceWait    time.Duration
	messageLifetime time.Duration

	width, height      int
	matcher            keys.Matcher
	sequenceGeneration int

	showHelp bool
	confirm  *confirmPrompt // open yes/no question, or nil
	cmdline  commandLine
	status   struct {
		text       string
		kind       statusKind
		identifier int
	}
	spinning bool // the root spinner's tick chain is running
	spin     spinner.Model
}

// New returns a root model whose stack holds just root.
func New(root View, theme theme.Theme, options Options) *Model {
	if options.SequenceTimeout == 0 {
		options.SequenceTimeout = DefaultSequenceTimeout
	}
	if options.MessageLifetime == 0 {
		options.MessageLifetime = DefaultMessageLifetime
	}
	if options.Tick == nil {
		options.Tick = tea.Tick
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Model{
		stack:           []View{root},
		th:              theme,
		keys:            keys.DefaultGlobal(),
		log:             options.Logger,
		tick:            options.Tick,
		sequenceWait:    options.SequenceTimeout,
		messageLifetime: options.MessageLifetime,
		cmdline:         newCommandLine(theme),
		spin:            spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner)),
	}
}

// Init starts the root view and asks the terminal for its background colour so
// the theme can match light and dark terminals.
func (model *Model) Init() tea.Cmd {
	return tea.Batch(model.top().Init(), tea.RequestBackgroundColor)
}

func (model *Model) top() View { return model.stack[len(model.stack)-1] }

func (model *Model) contentSize() (width, height int) { return model.width, max(0, model.height-1) }

// Update implements tea.Model.
func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	command := model.update(message)
	return model, tea.Batch(command, model.syncSpinner())
}

func (model *Model) update(message tea.Msg) tea.Cmd {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
		model.cmdline.input.SetWidth(max(1, message.Width-2)) // room for the prompt and cursor
		return model.broadcast(model.sizeMessage())
	case tea.BackgroundColorMsg:
		model.th = theme.New(message.IsDark())
		model.cmdline.setTheme(model.th)
		model.spin.Style = model.th.Spinner
		return model.broadcast(ThemeMessage{Theme: model.th})
	case tea.KeyPressMsg:
		return model.handleKey(message)
	case PushMessage:
		return model.push(message.View)
	case PopMessage:
		return model.pop()
	case QuitMessage:
		return tea.Quit
	case ConfirmMessage:
		model.resetKeys()
		model.confirm = newConfirmPrompt(message)
		return nil
	case InfoMessage:
		return model.setStatus(message.Text, statusInfo)
	case ErrorMessage:
		model.log.Warn("error shown to user", "err", message.Err)
		text := "error"
		if message.Err != nil {
			text = message.Err.Error()
		}
		return model.setStatus(text, statusError)
	case clearStatusMessage:
		if message.identifier == model.status.identifier {
			model.status.text, model.status.kind = "", statusNone
		}
		return nil
	case sequenceTimeoutMessage:
		return model.expireSequence(message.generation)
	case spinner.TickMsg:
		if message.ID != model.spin.ID() {
			return model.broadcastOther(message) // a view's own spinner
		}
		if !model.spinning {
			return nil // let the tick chain die while idle
		}
		var command tea.Cmd
		model.spin, command = model.spin.Update(message)
		return command
	default:
		return model.broadcastOther(message)
	}
}

func (model *Model) sizeMessage() tea.WindowSizeMsg {
	width, height := model.contentSize()
	return tea.WindowSizeMsg{Width: width, Height: height}
}

// broadcast sends msg to every view in the stack.
func (model *Model) broadcast(message tea.Msg) tea.Cmd {
	commands := make([]tea.Cmd, len(model.stack))
	for i, view := range model.stack {
		model.stack[i], commands[i] = view.Update(message)
	}
	return tea.Batch(commands...)
}

// broadcastOther sends a message the root does not consume to every view (and
// to the command line, for cursor blink).
func (model *Model) broadcastOther(message tea.Msg) tea.Cmd {
	if model.cmdline.active {
		return tea.Batch(model.cmdline.update(message), model.broadcast(message))
	}
	return model.broadcast(message)
}

func (model *Model) forward(message tea.Msg) tea.Cmd {
	view, command := model.top().Update(message)
	model.stack[len(model.stack)-1] = view
	return command
}

func (model *Model) push(view View) tea.Cmd {
	if view == nil {
		return nil
	}
	model.resetKeys()
	model.stack = append(model.stack, view)
	model.log.Debug("push view", "title", view.Title(), "depth", len(model.stack))
	return tea.Batch(view.Init(), model.forward(model.sizeMessage()))
}

func (model *Model) pop() tea.Cmd {
	model.resetKeys()
	if len(model.stack) == 1 {
		return tea.Quit
	}
	model.stack = model.stack[:len(model.stack)-1]
	model.log.Debug("pop view", "depth", len(model.stack))
	if review, ok := model.top().(Resumer); ok {
		return review.Resumed()
	}
	return nil
}

func (model *Model) resetKeys() {
	model.showHelp = false
	model.confirm = nil
	model.cmdline.close()
	model.matcher.Reset()
	model.sequenceGeneration++
}

func (model *Model) setStatus(text string, kind statusKind) tea.Cmd {
	model.status.identifier++
	model.status.text, model.status.kind = text, kind
	identifier := model.status.identifier
	return model.tick(model.messageLifetime, func(time.Time) tea.Msg { return clearStatusMessage{identifier: identifier} })
}

// syncSpinner starts the root spinner's tick chain when the top view is
// loading and lets it die when it is not. It runs after every Update, so
// push, pop and view state changes are all covered.
func (model *Model) syncSpinner() tea.Cmd {
	loader, ok := model.top().(Loader)
	want := ok && loader.Loading()
	switch {
	case want && !model.spinning:
		model.spinning = true
		return model.spin.Tick
	case !want:
		model.spinning = false
	}
	return nil
}

// bindings returns every binding active now, for sequence matching.
func (model *Model) bindings() []key.Binding {
	out := model.keys.Bindings()
	for _, bindings := range model.top().FullHelp() {
		out = append(out, bindings...)
	}
	return out
}

func (model *Model) handleKey(message tea.KeyPressMsg) tea.Cmd {
	k := message.String()
	if confirmPrompt := model.confirm; confirmPrompt.CapturesKeys() {
		model.confirm = nil
		return confirmPrompt.answer(message)
	}
	if model.cmdline.active {
		line, ok, command := model.cmdline.handleKey(message)
		if ok {
			return parseCommand(line, model.top())
		}
		return command
	}
	if model.showHelp {
		if key.Matches(message, model.keys.Help, model.keys.Cancel) {
			model.showHelp = false
		}
		return nil
	}
	if capturer, ok := model.top().(KeyCapturer); ok && capturer.CapturesKeys() {
		model.resetKeys()
		return model.forward(message)
	}

	result := model.matcher.Feed(k, model.bindings())
	model.sequenceGeneration++
	switch result.Kind {
	case keys.Pending:
		generation := model.sequenceGeneration
		return model.tick(model.sequenceWait, func(time.Time) tea.Msg { return sequenceTimeoutMessage{generation: generation} })
	case keys.Matched:
		return model.dispatch(result.Sequence)
	default:
		return model.forward(message)
	}
}

func (model *Model) expireSequence(generation int) tea.Cmd {
	if generation != model.sequenceGeneration || !model.matcher.Pending() {
		return nil
	}
	model.sequenceGeneration++
	if result := model.matcher.Expire(model.bindings()); result.Kind == keys.Matched {
		return model.dispatch(result.Sequence)
	}
	return nil
}

// dispatch acts on a resolved key or sequence: the root consumes Help and
// Command, everything else goes to the view.
func (model *Model) dispatch(sequence string) tea.Cmd {
	keyMessage := keys.Message{Sequence: sequence}
	switch {
	case key.Matches(keyMessage, model.keys.Help):
		model.showHelp = true
		return nil
	case key.Matches(keyMessage, model.keys.Command):
		return model.cmdline.open()
	}
	return model.forward(keyMessage)
}

// View implements tea.Model.
func (model *Model) View() tea.View {
	var view tea.View
	if model.width > 0 && model.height > 0 {
		view = tea.NewView(model.content() + "\n" + model.bottomRow())
	}
	view.AltScreen = true
	return view
}

// content renders the area above the status bar, padded to full size.
func (model *Model) content() string {
	width, height := model.contentSize()
	var text string
	if model.showHelp {
		text = renderHelp(model.th, model.top().Title(), model.keys.FullHelp(), model.top().FullHelp(), width)
	} else {
		text = model.top().Render(width, height)
	}
	text = model.th.Normal.MaxWidth(width).MaxHeight(height).Render(text)
	if count := height - lipgloss.Height(text); count > 0 {
		text += strings.Repeat("\n", count)
	}
	return text
}

func (model *Model) bar() statusBar {
	crumbs := make([]string, len(model.stack))
	for i, view := range model.stack {
		crumbs[i] = view.Title()
	}
	currentStatusBar := statusBar{
		crumbs:  strings.Join(crumbs, " › "),
		text:    model.status.text,
		kind:    model.status.kind,
		pending: model.matcher.Display(),
	}
	if currentStatusBar.kind == statusNone {
		currentStatusBar.text = shortHelp(model.top().ShortHelp())
	}
	if model.spinning {
		currentStatusBar.spinner = model.spin.View()
	}
	return currentStatusBar
}

func shortHelp(bindings []key.Binding) string {
	parts := []string{"? help"}
	for _, binding := range bindings {
		if help := binding.Help(); binding.Enabled() && help.Desc != "" {
			parts = append(parts, help.Key+" "+help.Desc)
		}
	}
	return strings.Join(parts, "  ")
}

// bottomRow is the status bar, or the command line while it is open.
func (model *Model) bottomRow() string {
	if model.confirm != nil {
		return model.th.StatusInfo.Width(model.width).MaxWidth(model.width).MaxHeight(1).Render(model.confirm.render(model.width))
	}
	if model.cmdline.active {
		return model.th.StatusCommand.Width(model.width).MaxWidth(model.width).MaxHeight(1).Render(model.cmdline.input.View())
	}
	return model.bar().render(model.th, model.width)
}
