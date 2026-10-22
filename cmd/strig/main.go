// Command strig is a terminal UI for reviewing GitHub pull requests.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/api"
	"github.com/MikePors/strigidae/internal/gitrepo"
	"github.com/MikePors/strigidae/internal/logging"
	"github.com/MikePors/strigidae/internal/version"
)

// deps are the side effects run needs, injected so tests exercise flag
// handling and repository inference without a checkout, the network or a
// terminal.
type deps struct {
	getwd  func() (string, error)
	getenv func(string) string
	home   func() (string, error)
	runner gitrepo.Runner
	// knownHost reports whether host is a GitHub host gh can talk to.
	knownHost func(host string) bool
	newClient func(host string) (github.Client, error)
	start     func(a app, stderr io.Writer) error
}

func realDeps() deps {
	return deps{
		getwd:     os.Getwd,
		getenv:    os.Getenv,
		home:      os.UserHomeDir,
		runner:    gitrepo.ExecRunner,
		knownHost: api.KnownHost,
		newClient: func(host string) (github.Client, error) {
			c, err := api.New(api.Options{Host: host})
			if err != nil {
				return nil, err // not c: a nil *Client would be a non-nil interface
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
	return report(stderr, execute(args, stdout, stderr, d))
}

func execute(args []string, stdout, stderr io.Writer, d deps) error {
	fs := flag.NewFlagSet("strig", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print version and exit")
	showLogPath := fs.Bool("log-path", false, "print the log file path and exit")
	showConfigPath := fs.Bool("config-path", false, "print the config file path and exit")
	configFlag := fs.String("config", "", "config file `path` (default: $XDG_CONFIG_HOME/strig/config.yml)")
	repoFlag := fs.String("R", "", "repository to show as `owner/repo` (default: the current checkout's upstream or origin remote)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return &usageError{} // the flag package has printed the problem and usage
	}
	if fs.NArg() > 0 {
		return &usageError{fmt.Sprintf("unexpected argument %q (did you mean -R %s?)", fs.Arg(0), fs.Arg(0))}
	}

	configPath, configErr := configLocation(*configFlag, d)

	switch {
	case *showVersion:
		return printLine(stdout, "strig "+version.String())
	case *showLogPath:
		path, err := logging.Path(os.Getenv, os.UserHomeDir)
		if err != nil {
			return err
		}
		return printLine(stdout, path)
	case *showConfigPath:
		if configErr != nil {
			return configErr
		}
		return printLine(stdout, configPath)
	}

	if configErr != nil {
		return configErr
	}
	cfg, err := config.Load(configPath, *configFlag != "")
	if err != nil {
		return err
	}
	a, err := newApp(*repoFlag, cfg, d)
	if err != nil {
		return err
	}
	return d.start(a, stderr)
}

// configLocation is the --config path if given, else the default location.
func configLocation(flagValue string, d deps) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	return config.Path(d.getenv, d.home)
}

func printLine(stdout io.Writer, s string) error {
	_, err := fmt.Fprintln(stdout, s)
	return err
}
