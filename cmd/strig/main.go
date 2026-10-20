// Command strig is a terminal UI for reviewing GitHub pull requests.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"
	cliBrowser "github.com/cli/browser"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/api"
	"github.com/MikePors/strigidae/internal/gitrepo"
	"github.com/MikePors/strigidae/internal/logging"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
	"github.com/MikePors/strigidae/internal/ui/views/prlist"
	"github.com/MikePors/strigidae/internal/version"
)

// deps are the side effects run needs, injected so tests exercise flag
// handling and repository inference without a checkout, the network or a
// terminal.
type deps struct {
	getwd  func() (string, error)
	runner gitrepo.Runner
	// knownHost reports whether host is a GitHub host gh can talk to.
	knownHost func(host string) bool
	newClient func(host string) (github.Client, error)
	start     func(client github.Client, repo github.Repo, stderr io.Writer) error
}

func realDeps() deps {
	return deps{
		getwd:  os.Getwd,
		runner: gitrepo.ExecRunner,
		knownHost: api.KnownHost,
		newClient: func(host string) (github.Client, error) {
			c, err := api.New(api.Options{Host: host})
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		start: runTUI,
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, realDeps()))
}

func run(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("strig", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print version and exit")
	showLogPath := fs.Bool("log-path", false, "print the log file path and exit")
	repoFlag := fs.String("R", "", "repository to show as `owner/repo` (default: the current checkout's upstream or origin remote)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "strig: unexpected argument %q (did you mean -R %s?)\n", fs.Arg(0), fs.Arg(0)) // best effort
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

	target, code := resolveTarget(*repoFlag, stderr, d)
	if code != 0 {
		return code
	}
	client, err := d.newClient(target.Host)
	if errors.Is(err, github.ErrNotAuthenticated) {
		login := "gh auth login"
		if target.Host != "" {
			login += " --hostname " + target.Host
		}
		_, _ = fmt.Fprintf(stderr, "strig: not signed in to %s: run `%s` and try again\n", cmp.Or(target.Host, "GitHub"), login) // best effort, before the TUI starts
		return 1
	}
	if err != nil {
		return fail(stderr, err)
	}
	if err := d.start(client, target.Repo, stderr); err != nil {
		return fail(stderr, err)
	}
	return 0
}

// resolveTarget decides which repository to show: -R if given, else the
// current checkout's remote. A nonzero code means run should exit with it.
func resolveTarget(repoFlag string, stderr io.Writer, d deps) (gitrepo.Remote, int) {
	if repoFlag != "" {
		repo, err := github.ParseRepo(repoFlag)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "strig: -R: %v\n", err) // best effort
			return gitrepo.Remote{}, 2
		}
		// No host: the client uses the gh default (GH_HOST or github.com).
		return gitrepo.Remote{Repo: repo}, 0
	}
	dir, err := d.getwd()
	if err != nil {
		return gitrepo.Remote{}, fail(stderr, err)
	}
	remote, err := gitrepo.Infer(context.Background(), d.runner, dir, d.knownHost)
	var unknown *gitrepo.UnknownHostError
	switch {
	case err == nil:
		return remote, 0
	case errors.As(err, &unknown):
		_, _ = fmt.Fprintf(stderr, "strig: remote host %s isn't a GitHub host gh is signed in to; pass -R owner/repo or run gh auth login --hostname <host>\n", unknown.QuotedHosts()) // best effort
	case errors.Is(err, gitrepo.ErrNotCheckout):
		_, _ = fmt.Fprintln(stderr, "strig: run strig inside a GitHub repository checkout, or pass -R owner/repo") // best effort
	case errors.Is(err, gitrepo.ErrNoGitHubRemote):
		_, _ = fmt.Fprintln(stderr, "strig: no GitHub remote (upstream or origin) in this checkout; pass -R owner/repo") // best effort
	default:
		return gitrepo.Remote{}, fail(stderr, err)
	}
	return gitrepo.Remote{}, 1
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
func runTUI(client github.Client, repo github.Repo, stderr io.Writer) error {
	logger, closeLog := openLog(stderr)
	defer closeLog()
	logging.Install(logger)
	silenceBrowser()
	logger.Info("starting", "version", version.String(), "repo", repo.String())

	th := theme.Default()
	model := ui.New(prlist.New(client, repo, th, prlist.Options{}), th, ui.Options{Logger: logger})
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
