package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/config"

	"github.com/MikePors/strigidae/internal/browseropener"
	"github.com/MikePors/strigidae/internal/clipboard"
	"github.com/MikePors/strigidae/internal/environment"
	"github.com/MikePors/strigidae/internal/logging"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme/terminalpalette"
	"github.com/MikePors/strigidae/internal/userinterface/views/dashboard"
	"github.com/MikePors/strigidae/internal/userinterface/views/pullrequestlist"
	"github.com/MikePors/strigidae/internal/userinterface/views/workspace"
	"github.com/MikePors/strigidae/internal/version"
)

// runTerminalInterface starts the interactive program. Logs go to a file because the TUI
// owns the terminal; if the file can't be opened we warn once and carry on
// with logging discarded.
func runTerminalInterface(application application, stderr io.Writer) error {
	logger, closeLog, err := logging.Start(os.Getenv, os.UserHomeDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "strig: logging disabled: %v\n", err) // best effort, before the TUI starts
	}
	defer closeLog()
	logging.Install(logger)
	logger.Info("starting", "version", version.String(), "repo", application.repository.String())

	built, notice := application.buildTheme(logger, askTerminal)
	application.theme = built
	model := userinterface.New(firstView(application), application.theme, application.userInterfaceOptions(logger, notice))
	if err := runProgram(tea.NewProgram(model), model); err != nil {
		logger.Error("program failed", "err", err)
		return fmt.Errorf("run: %w", err)
	}
	logger.Info("exiting")
	return nil
}

// userInterfaceOptions is what the shell starts with: the logger, the startup
// notice, and the options the config file set.
func (application application) userInterfaceOptions(logger *slog.Logger, notice string) userinterface.Options {
	runner, commands, names := application.pluginOptions(logger)
	// Without a state directory local threads live for the session only.
	stateDirectory, err := logging.StateDirectory(os.Getenv, os.UserHomeDir)
	if err != nil {
		logger.Warn("local threads are not kept between runs", "err", err)
	}
	return userinterface.Options{
		StateDirectory: stateDirectory,
		Logger:         logger, Notice: notice, StartingOptions: application.config.Options,
		ViewCommands: append(viewCommands(), names...),
		PluginRunner: runner, PluginCommands: commands,
	}
}

// viewCommands collects the command names every view package declares, so the
// shell can tell a command another view owns from a word that is no command.
// A view with commands adds its Commands here. A name more than one view
// handles (":copy-url") is listed once.
func viewCommands() []string {
	var names []string
	for _, name := range slices.Concat(workspace.Commands(), pullrequestlist.Commands(), dashboard.Commands()) {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// askTerminal asks the controlling terminal for its palette, before the
// program takes over the terminal.
func askTerminal() (terminalpalette.Reply, error) {
	controllingTerminal, err := terminalpalette.Open()
	if err != nil {
		return terminalpalette.Reply{}, err
	}
	reply, err := terminalpalette.Ask(controllingTerminal, os.Getenv("TMUX") != "", time.Now)
	return reply, errors.Join(err, controllingTerminal.Close())
}

// firstView is the dashboard when there is no repository (outside a checkout,
// or in one with no GitHub remote), else the repository's pull request list.
func firstView(application application) userinterface.View {
	if application.dashboard {
		return dashboard.New(application.client, application.config.Tabs, application.theme, dashboard.Options{Cache: application.cache, Notice: application.notice, Timeouts: application.timeouts(), Clipboard: systemClipboard(), Open: systemBrowser().Open})
	}
	options := pullrequestlist.Options{Cache: application.cache, Timeouts: application.timeouts(), Clipboard: systemClipboard(), Open: systemBrowser().Open, CurrentBranch: application.currentBranch}
	if application.blobs != nil { // a nil pointer must not become a non-nil interface
		options.Blobs = application.blobs
	}
	return pullrequestlist.New(application.client, application.repository, application.config.Tabs, application.theme, options)
}

// systemClipboard is the real clipboard tool lookup.
func systemClipboard() clipboard.Copier {
	return clipboard.New(clipboardOptions(os.Environ()))
}

// clipboardOptions gives the tool the same scrubbed environment as every
// program strig starts.
func clipboardOptions(variables []string) clipboard.Options {
	return clipboard.Options{
		Platform:       runtime.GOOS,
		GetEnvironment: os.Getenv,
		LookPath:       exec.LookPath,
		Environment:    environment.Scrub(variables),
	}
}

// timeouts hands the config file's deadlines to the views; a field the file
// left out is zero, which selects the view's default.
func (application application) timeouts() userinterface.Timeouts {
	return userinterface.Timeouts{Request: application.config.Timeouts.Request, Load: application.config.Timeouts.Load}
}

// systemBrowser is the real browser opener.
func systemBrowser() *browseropener.Opener {
	return browseropener.New(browserOptions(os.Environ(), ghConfigBrowser))
}

// browserOptions gives the launcher the same scrubbed environment as every
// program strig starts.
func browserOptions(variables []string, configuredBrowser func() string) browseropener.Options {
	return browseropener.Options{
		Platform:          runtime.GOOS,
		GetEnvironment:    os.Getenv,
		ConfiguredBrowser: configuredBrowser,
		LookPath:          exec.LookPath,
		Lstat:             os.Lstat,
		Environment:       environment.Scrub(variables),
	}
}

// ghConfigBrowser is the browser setting of the gh CLI's config, or "" when
// the config can't be read or sets none.
func ghConfigBrowser() string {
	configuration, err := config.Read(nil)
	if err != nil {
		return ""
	}
	browser, _ := configuration.Get([]string{"browser"})
	return browser
}

// runProgram runs the program and then ends the plugin runs, on every path
// out of Run: a signal ends Bubble Tea without the model seeing a quit.
func runProgram(program *tea.Program, model *userinterface.Model) error {
	_, err := program.Run()
	model.Close()
	return err
}
