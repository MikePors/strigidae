// Command strig is a terminal UI for reviewing GitHub pull requests.
package main

import (
	"errors"
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
	showLogPath := fs.Bool("log-path", false, "print the log file path and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	switch {
	case *showVersion:
		return printLine(stdout, stderr, "strig "+version.String())
	case *showLogPath:
		path, err := logging.Path(os.Getenv, os.UserHomeDir)
		if err != nil {
			return fail(stderr, err)
		}
		return printLine(stdout, stderr, path)
	}

	if err := runTUI(stderr); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func printLine(stdout, stderr io.Writer, s string) int {
	if _, err := fmt.Fprintln(stdout, s); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "strig: %v\n", err) // best effort: nowhere else to report
	return 1
}

// runTUI starts the interactive program. Logs go to a file because the TUI
// owns the terminal; if the file can't be opened we warn once and carry on
// with logging discarded.
func runTUI(stderr io.Writer) error {
	logger, closeLog := openLog(stderr)
	defer closeLog()
	logging.Install(logger)
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

func openLog(stderr io.Writer) (*slog.Logger, func()) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	path, err := logging.Path(os.Getenv, os.UserHomeDir)
	if err == nil {
		var l *slog.Logger
		var f *os.File
		if l, f, err = logging.Open(path); err == nil {
			return l, func() { _ = f.Close() }
		}
	}
	_, _ = fmt.Fprintf(stderr, "strig: logging disabled: %v\n", err) // best effort, before the TUI starts
	return discard, func() {}
}
