// Command strig is a terminal UI for reviewing GitHub pull requests.
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/logging"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
	"github.com/MikePors/strigidae/internal/ui/views/placeholder"
	"github.com/MikePors/strigidae/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("strig", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintf(stdout, "strig %s\n", version.String())
		return 0
	}

	if err := runTUI(stderr); err != nil {
		fmt.Fprintf(stderr, "strig: %v\n", err)
		return 1
	}
	return 0
}

// runTUI starts the interactive program. Logs go to a file because the TUI
// owns the terminal; if the file can't be opened we warn once and carry on.
func runTUI(stderr io.Writer) error {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if path, err := logging.Path(os.Getenv, os.UserHomeDir); err == nil {
		l, f, err := logging.Open(path)
		if err != nil {
			fmt.Fprintf(stderr, "strig: logging disabled: %v\n", err)
		} else {
			defer func() { _ = f.Close() }()
			logger = l
		}
	}
	logger.Info("starting", "version", version.String())

	th := theme.Default()
	model := ui.New(placeholder.New(th, 1), th, ui.Options{Logger: logger})
	if _, err := tea.NewProgram(model).Run(); err != nil {
		logger.Error("program failed", "err", err)
		return fmt.Errorf("run: %w", err)
	}
	logger.Info("exiting")
	return nil
}
