package main

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	cliBrowser "github.com/cli/browser"

	"github.com/MikePors/strigidae/internal/logging"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/views/prlist"
	"github.com/MikePors/strigidae/internal/version"
)

// runTUI starts the interactive program. Logs go to a file because the TUI
// owns the terminal.
func runTUI(a app, stderr io.Writer) error {
	logger, closeLog := logging.Start(stderr, os.Getenv, os.UserHomeDir)
	defer closeLog()
	logging.Install(logger)
	silenceBrowser()
	logger.Info("starting", "version", version.String(), "repo", a.repo.String())

	model := ui.New(prlist.New(a.client, a.repo, a.theme, prlist.Options{}), a.theme, ui.Options{Logger: logger})
	if _, err := tea.NewProgram(model).Run(); err != nil {
		logger.Error("program failed", "err", err)
		return fmt.Errorf("run: %w", err)
	}
	logger.Info("exiting")
	return nil
}

// silenceBrowser stops browser launchers writing to the terminal the TUI owns.
// When no launcher is configured, go-gh falls back to cli/browser, which wires
// xdg-open and friends to these package globals (os.Stdout/os.Stderr by
// default) regardless of the writers given to browser.New.
func silenceBrowser() {
	cliBrowser.Stdout = io.Discard
	cliBrowser.Stderr = io.Discard
}
