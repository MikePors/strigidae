package main

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	cliBrowser "github.com/cli/browser"

	"github.com/MikePors/strigidae/internal/logging"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/views/dashboard"
	"github.com/MikePors/strigidae/internal/userinterface/views/pullrequestlist"
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
	silenceBrowser()
	logger.Info("starting", "version", version.String(), "repo", application.repository.String())

	application.theme = application.buildTheme(logger)
	model := userinterface.New(firstView(application), application.theme, userinterface.Options{Logger: logger})
	if _, err := tea.NewProgram(model).Run(); err != nil {
		logger.Error("program failed", "err", err)
		return fmt.Errorf("run: %w", err)
	}
	logger.Info("exiting")
	return nil
}

// firstView is the dashboard when there is no repository (outside a checkout,
// or in one with no GitHub remote), else the repository's pull request list.
func firstView(application application) userinterface.View {
	if application.dashboard {
		return dashboard.New(application.client, application.config.Tabs, application.theme, dashboard.Options{Notice: application.notice})
	}
	return pullrequestlist.New(application.client, application.repository, application.theme, pullrequestlist.Options{})
}

// silenceBrowser stops browser launchers writing to the terminal the TUI owns.
// When no launcher is configured, go-gh falls back to cli/browser, which wires
// xdg-open and friends to these package globals (os.Stdout/os.Stderr by
// default) regardless of the writers given to browser.New.
func silenceBrowser() {
	cliBrowser.Stdout = io.Discard
	cliBrowser.Stderr = io.Discard
}
