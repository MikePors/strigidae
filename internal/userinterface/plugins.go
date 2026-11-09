package userinterface

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/localthreads"
	"github.com/MikePors/strigidae/internal/plugins"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// DefaultPluginQuitWait is the longest strig waits for plugin runs to end
// after asking to quit. The runs are cancelled at once; this only bounds
// what a plugin or the runner could still take.
const DefaultPluginQuitWait = 2 * time.Second

// PluginRunner runs the plugin called name. *plugins.Runner is the real one.
type PluginRunner interface {
	Run(ctx context.Context, name string, request plugins.Request) ([]plugins.Effect, error)
}

// PluginCommand is a configured plugin as the views see it: a command name
// and what the command acts on.
type PluginCommand struct {
	Name   string
	Target config.Target
	// Show is where the answer is drawn; empty is the status bar.
	Show config.Show
}

// PluginsMessage tells views which plugin commands exist. Like OptionsMessage,
// the root sends it to the root view when strig starts and to every view it
// pushes.
type PluginsMessage struct{ Commands []PluginCommand }

// PullRequestIdentity names a pull request at one head commit. It is carried
// by a plugin run and compared (repository and number) with the pull request
// on screen when the run ends, so a result is shown only where it belongs.
type PullRequestIdentity struct {
	// Host is the GitHub host, empty if the view does not know it.
	Host       string
	Repository github.Repository
	Number     int
	Head       string
}

// SamePullRequest reports whether both name the same pull request, whatever
// their heads. Result routing and the spinner both use it, so they agree: a
// new commit during a run does not hide its result, and the plugin has the
// head it was run at in its request.
func (identity PullRequestIdentity) SamePullRequest(other PullRequestIdentity) bool {
	return identity.Repository == other.Repository && identity.Number == other.Number
}

// RunPluginMessage asks the root to run a plugin. Runs belong to the root,
// not to the view that asked: closing the view does not cancel one, and
// quitting does.
type RunPluginMessage struct {
	Name    string
	Request plugins.Request
	// Identity is the pull request the run was asked from.
	Identity PullRequestIdentity
}

// PluginHost is implemented by a view that shows a pull request and so can
// receive the effects of a plugin run.
type PluginHost interface {
	// PullRequestIdentity returns the pull request on screen; ok is false until
	// it has loaded.
	PullRequestIdentity() (identity PullRequestIdentity, ok bool)
	// Reload reloads what the view shows, as its refresh key does.
	Reload() tea.Cmd
	// PluginsMayRun reports whether a plugin's command effect may run the
	// view's command name, which Command then runs. Only commands that write
	// nothing to GitHub may be allowed.
	PluginsMayRun(name string) bool
	Commander
}

// CommandLineKeeper is implemented by a KeyCapturer that lets ":" open the
// command line while it captures keys (a line selection that a command is to
// use). The line closes without touching the view's state, so cancelling it
// leaves the view as it was.
type CommandLineKeeper interface {
	KeepsCommandLine() bool
}

// pluginStopper is a runner with processes that outlive a run, the services
// of *plugins.Runner: Close stops them alongside cancelling the runs.
type pluginStopper interface{ Stop() }

// pluginRun is a run the root has started and not yet heard back from.
type pluginRun struct {
	identity PullRequestIdentity
	name     string
	// thread is the local thread the run fills, or "" when it has none.
	thread string
}

type (
	pluginResultMessage struct {
		run     int
		effects []plugins.Effect
		err     error
	}
	// quitWaitedMessage ends the wait for plugin runs at quit.
	quitWaitedMessage struct{}
)

// startPlugin begins a run in a command the root owns. After quit began there
// are no new runs.
func (model *Model) startPlugin(message RunPluginMessage) tea.Cmd {
	if model.quitting || model.pluginRunner == nil {
		return nil
	}
	if model.pluginContext == nil {
		model.pluginContext, model.pluginCancel = context.WithCancel(context.Background())
	}
	model.pluginSequence++
	run := model.pluginSequence
	started := pluginRun{identity: message.Identity, name: message.Name}
	var shown tea.Cmd
	if model.showsThread(message.Name) && message.Request.Target != nil {
		started.thread = model.askLocalThread(message)
		shown = tea.Batch(model.broadcast(LocalThreadsMessage{Threads: model.localThreads.Snapshot()}), model.saveLocalThreads(model.localThreadKey(message)))
	}
	model.pluginRuns[run] = started
	runner, ctx := model.pluginRunner, model.pluginContext
	model.pluginRunning.Add(1)
	return tea.Batch(shown, func() tea.Msg {
		defer model.pluginRunning.Done()
		effects, err := runner.Run(ctx, message.Name, message.Request)
		return pluginResultMessage{run: run, effects: effects, err: err}
	})
}

// showsThread reports whether the plugin draws its answer as a local thread.
func (model *Model) showsThread(name string) bool {
	for _, command := range model.pluginCommands {
		if command.Name == name {
			return command.Show == config.ShowThread
		}
	}
	return false
}

// askLocalThread adds the thread a run will fill, at once, so it shows even
// while the run waits its turn. The question is the arguments, or the command
// alone when there are none.
func (model *Model) askLocalThread(message RunPluginMessage) string {
	target := message.Request.Target
	question := message.Request.Arguments
	if question == "" {
		question = ":" + message.Name
	}
	return model.localThreads.Add(LocalThread{
		Host:       message.Request.Repository.Host,
		Asked:      model.clock(),
		Repository: message.Identity.Repository,
		Number:     message.Identity.Number,
		Head:       message.Request.Head,
		Path:       target.Path,
		Side:       github.DiffSide(target.Side),
		Line:       target.Line,
		StartLine:  target.StartLine,
		Question:   question,
		State:      github.LocalWaiting,
	})
}

// localThreadKey is the pull request an asked thread belongs to. Its host is
// the one in the request, the same the plugin is given.
func (model *Model) localThreadKey(message RunPluginMessage) localthreads.Key {
	return localthreads.Key{Host: message.Request.Repository.Host, Repository: message.Identity.Repository, Number: message.Identity.Number}
}

// finishLocalThread fills the thread of a finished run: the answers joined by
// a blank line, or why there is none. An error effect with no answer is the
// reason; a run that printed neither says so. The status bar is left alone: the thread is
// where the outcome shows.
func (model *Model) finishLocalThread(identifier string, message pluginResultMessage) tea.Cmd {
	state, text := github.LocalFailed, ""
	var answers []string
	var failure string
	for _, effect := range message.effects {
		switch effect.Name {
		case plugins.EffectAnswer:
			answers = append(answers, effect.Text)
		case plugins.EffectError:
			if failure == "" {
				failure = effect.Text
			}
		}
	}
	switch {
	case message.err != nil && len(answers) == 0:
		text = message.err.Error()
	case len(answers) > 0:
		state, text = github.LocalAnswered, strings.Join(answers, "\n\n")
	case failure != "":
		text = failure
	default:
		text = "the plugin gave no answer"
	}
	var saved tea.Cmd
	if key, found := model.localThreads.Finish(identifier, state, text); found {
		saved = model.saveLocalThreads(key)
	}
	return tea.Batch(model.broadcast(LocalThreadsMessage{Threads: model.localThreads.Snapshot()}), saved)
}

// pluginActive reports whether a run of the pull request the top view shows
// is under way or queued.
func (model *Model) pluginActive() bool {
	host, ok := model.top().(PluginHost)
	if !ok || len(model.pluginRuns) == 0 {
		return false
	}
	shown, ok := host.PullRequestIdentity()
	if !ok {
		return false
	}
	for _, run := range model.pluginRuns {
		if run.identity.SamePullRequest(shown) {
			return true
		}
	}
	return false
}

// pluginResult shows the effects of a finished run on the pull request it was
// asked from. Anything else is dropped, with a log line that names no text.
func (model *Model) pluginResult(message pluginResultMessage) tea.Cmd {
	finished := model.pluginRuns[message.run]
	delete(model.pluginRuns, message.run)
	var filled tea.Cmd
	if finished.thread != "" && !model.quitting {
		// The thread is in the store whatever is on screen, so an answer that
		// comes while its pull request is closed is there when it reopens.
		filled = model.finishLocalThread(finished.thread, message)
	}
	if model.quitting {
		// Nothing arriving after quit began is shown. The last run to end
		// lets strig go before the wait is over.
		if len(model.pluginRuns) == 0 {
			return tea.Quit
		}
		return nil
	}
	host, ok := model.hostShowing(finished.identity)
	if !ok {
		model.log.Info("plugin result dropped: its pull request is not on screen", "plugin", finished.name, "number", finished.identity.Number)
		return filled
	}
	if message.err != nil {
		model.log.Warn("plugin failed", "plugin", finished.name, "err", message.err)
		if finished.thread != "" {
			return filled
		}
		return model.setStatus(terminaltext.Clean(message.err.Error()), statusError)
	}
	return tea.Batch(filled, model.pluginEffects(host, finished, message.effects))
}

// hostShowing returns the top view if it is a plugin host showing the pull
// request of identity.
func (model *Model) hostShowing(identity PullRequestIdentity) (PluginHost, bool) {
	host, ok := model.top().(PluginHost)
	if !ok {
		return nil, false
	}
	shown, ok := host.PullRequestIdentity()
	if !ok || !shown.SamePullRequest(identity) {
		return nil, false
	}
	return host, true
}

// pluginStatusMessage is a notice or error of a plugin, or of a command a
// plugin ran, that arrives after the result. It is shown only if the result's
// pull request is still on screen (identity) and strig is not quitting. It is
// never logged: the text may be the plugin's, already cleaned.
type pluginStatusMessage struct {
	plugin   string
	identity PullRequestIdentity
	text     string
	kind     statusKind
}

// pluginStatus shows a deferred notice or error, if its pull request is still
// on screen.
func (model *Model) pluginStatus(message pluginStatusMessage) tea.Cmd {
	if _, ok := model.hostShowing(message.identity); !ok || model.quitting {
		model.log.Info("plugin notice dropped: its pull request is not on screen", "plugin", message.plugin, "number", message.identity.Number)
		return nil
	}
	return model.setStatus(message.text, message.kind)
}

// pluginStep is one effect of a result with commands, applied in its turn: a
// command line, or a notice or error when command is "".
type pluginStep struct {
	command, arguments string
	text               string
	kind               statusKind
}

// pluginStepsMessage carries the effects of a result with commands that are
// still to be applied, one step per message.
type pluginStepsMessage struct {
	plugin   string
	identity PullRequestIdentity
	steps    []pluginStep
}

// pluginSteps applies the first step, if the result's pull request is still
// on screen, then asks for the rest. A command runs only now, so it acts on
// the pull request it was asked for, and its own notices are tied to that
// pull request too. The rest waits for the command (tea.Sequence), so a
// command's notice keeps its place in the order the plugin printed.
func (model *Model) pluginSteps(message pluginStepsMessage) tea.Cmd {
	host, ok := model.hostShowing(message.identity)
	if !ok || model.quitting {
		model.log.Info("plugin effects dropped: their pull request is not on screen", "plugin", message.plugin, "number", message.identity.Number)
		return nil
	}
	if len(message.steps) == 0 {
		return nil
	}
	step, rest := message.steps[0], message
	rest.steps = message.steps[1:]
	var next tea.Cmd
	if len(rest.steps) > 0 {
		next = messageCommand(rest)
	}
	if step.command == "" {
		// The status is set now; its timer must not hold up the rest.
		return tea.Batch(model.setStatus(step.text, step.kind), next)
	}
	command, _ := host.Command(step.command, step.arguments)
	return tea.Sequence(pluginScoped(command, message.plugin, message.identity), next)
}

// pluginScoped turns the notices and errors that command produces, directly or
// in a batch, into messages tied to the pull request of identity. Its other
// messages (the clipboard) pass unchanged.
func pluginScoped(command tea.Cmd, plugin string, identity PullRequestIdentity) tea.Cmd {
	if command == nil {
		return nil
	}
	return func() tea.Msg {
		switch message := command().(type) {
		case InfoMessage:
			return pluginStatusMessage{plugin: plugin, identity: identity, text: message.Text, kind: statusInfo}
		case ErrorMessage:
			text := "error"
			if message.Err != nil {
				text = message.Err.Error()
			}
			return pluginStatusMessage{plugin: plugin, identity: identity, text: text, kind: statusError}
		case tea.BatchMsg:
			scoped := make(tea.BatchMsg, len(message))
			for i, member := range message {
				scoped[i] = pluginScoped(member, plugin, identity)
			}
			return scoped
		default:
			return message
		}
	}
}

// pluginEffects applies the effects in the order the plugin printed them.
// Their text is the plugin's, so it is cleaned before the status bar. A
// command effect that is refused drops the whole result, so a plugin that
// tries to write can't hide the refusal behind the notices of its other
// effects. A result with commands is applied step by step (pluginSteps); a
// reload sets no status and runs beside the steps.
func (model *Model) pluginEffects(host PluginHost, finished pluginRun, effects []plugins.Effect) tea.Cmd {
	name, inThread := finished.name, finished.thread != ""
	for _, effect := range effects {
		if effect.Name != plugins.EffectCommand {
			continue
		}
		if refusal := model.pluginCommandRefusal(host, name, effect.Line); refusal != "" {
			model.log.Info("plugin result dropped: a command was refused", slog.String("plugin", name))
			return model.setStatus(refusal, statusError)
		}
	}
	inOrder := slices.ContainsFunc(effects, func(effect plugins.Effect) bool { return effect.Name == plugins.EffectCommand })
	var commands []tea.Cmd
	var steps []pluginStep
	status := func(text string, kind statusKind) {
		if inOrder {
			steps = append(steps, pluginStep{text: text, kind: kind})
			return
		}
		commands = append(commands, model.setStatus(text, kind))
	}
	answered := slices.ContainsFunc(effects, func(effect plugins.Effect) bool { return effect.Name == plugins.EffectAnswer })
	for _, effect := range effects {
		// The thread shows the answers, and the error when there is no answer;
		// an error beside an answer would otherwise be lost, so it also goes
		// to the status bar.
		if inThread && (effect.Name == plugins.EffectAnswer || (effect.Name == plugins.EffectError && !answered)) {
			continue
		}
		switch effect.Name {
		case plugins.EffectNotice, plugins.EffectAnswer: // answers show as notices until they have a place of their own
			status(terminaltext.Clean(effect.Text), statusInfo)
		case plugins.EffectError:
			status(terminaltext.Clean(effect.Text), statusError)
		case plugins.EffectReload:
			commands = append(commands, host.Reload())
		case plugins.EffectCommand:
			commandName, arguments := splitPluginCommand(effect.Line)
			steps = append(steps, pluginStep{command: commandName, arguments: arguments})
		default:
			model.log.Warn("plugin effect not handled", slog.String("plugin", name), slog.String("effect", effect.Name))
		}
	}
	if len(steps) > 0 {
		commands = append(commands, model.pluginSteps(pluginStepsMessage{plugin: name, identity: finished.identity, steps: steps}))
	}
	return tea.Batch(commands...)
}

// pluginCommandRefusal is what the status bar says when a plugin may not run
// the command line of a command effect, or "" when it may. A plugin may show
// and navigate, never write, so only the commands the host allows run. The
// shell's own commands never run, so a plugin can't quit or change the
// options. The line is the plugin's, so what is shown of it is cleaned.
func (model *Model) pluginCommandRefusal(host PluginHost, plugin, line string) string {
	name, _ := splitPluginCommand(line)
	shell := slices.Contains(CommandNames(), name)
	if !shell && host.PluginsMayRun(name) {
		return ""
	}
	if !shell && !slices.Contains(model.viewCommands, name) {
		return "not a command: " + terminaltext.Clean(name)
	}
	return "plugin " + plugin + " can't run :" + terminaltext.Clean(name)
}

// splitPluginCommand splits the line of a command effect as the command line
// does; a leading ":" is allowed.
func splitPluginCommand(line string) (name, arguments string) {
	_, name, _, arguments, _ = splitCommandLine(strings.TrimPrefix(strings.TrimSpace(line), ":"))
	return strings.TrimSpace(name), strings.TrimSpace(arguments)
}

// quit ends the program. With plugin runs under way they are cancelled and
// strig waits for them, but never longer than the quit wait; with none it
// quits at once. A second request does not wait again.
func (model *Model) quit() tea.Cmd {
	if model.quitting || len(model.pluginRuns) == 0 {
		return tea.Quit
	}
	model.quitting = true
	model.pluginCancel()
	return model.tick(model.pluginQuitWait, func(time.Time) tea.Msg { return quitWaitedMessage{} })
}

// Close cancels every plugin run, stops the runner's services, and writes the
// local threads that changed since their last write, all within one quit wait.
// The writes do not wait for the runs, and do not depend on any command having
// run. cmd/strig calls it after the program returns, on every path:
// an exit that never reaches Update (SIGTERM ends Bubble Tea with a QuitMsg that
// the model does not see; SIGINT and a failed Run are others) would otherwise
// leave plugins running, and a plugin has a session of its own. The wait is
// for the runs to end, not just for the cancel: the kill happens in the
// runner's goroutine, which strig must not outrun by exiting. The program has
// returned, so nothing else uses the model; the caller must not use it
// afterwards.
func (model *Model) Close() {
	timeout := time.After(model.pluginQuitWait)
	if model.pluginCancel != nil {
		model.pluginCancel()
	}
	stopped := model.stopServices()
	flushed := model.localFiles.Flush(&model.localThreads) // its file work runs in a goroutine, beside the wait for the runs
	for _, finished := range []<-chan struct{}{flushed, stopped} {
		select {
		case <-finished:
		case <-timeout:
			return
		}
	}
	if model.pluginCancel != nil {
		waitFor(&model.pluginRunning, timeout)
	}
}

// stopServices starts stopping the runner's services before any quit wait,
// so a stuck run or file write cannot prevent the kill from being attempted.
func (model *Model) stopServices() <-chan struct{} {
	stopped := make(chan struct{})
	stopper, ok := model.pluginRunner.(pluginStopper)
	if !ok {
		close(stopped)
		return stopped
	}
	go func() {
		stopper.Stop()
		close(stopped)
	}()
	return stopped
}

// waitFor waits for the group or the remainder of the quit timeout.
func waitFor(group *sync.WaitGroup, timeout <-chan time.Time) {
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-timeout:
	}
}
