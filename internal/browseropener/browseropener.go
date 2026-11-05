// Package browseropener opens a URL in the user's web browser. It starts
// every program with the environment it is given (the scrubbed one from
// internal/environment) and discards the program's output, which would
// otherwise land on the screen the TUI owns.
//
// The launcher is chosen the way the gh CLI chooses it: GH_BROWSER, then the
// gh config's browser setting, then BROWSER, then the platform's default.
package browseropener

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os/exec"
	"strings"

	"github.com/google/shlex"
)

// Options are the things an Opener depends on, injected so tests never read
// the real environment or gh config, or start a real program.
type Options struct {
	// Platform is runtime.GOOS.
	Platform string
	// GetEnvironment looks up one environment variable (os.Getenv).
	GetEnvironment func(name string) string
	// ConfiguredBrowser returns the gh config's browser setting, or "" when
	// there is none. It is called only when GH_BROWSER is unset.
	ConfiguredBrowser func() string
	// LookPath finds a program on PATH (exec.LookPath).
	LookPath func(name string) (string, error)
	// Lstat describes a path without following symlinks (os.Lstat).
	Lstat func(name string) (fs.FileInfo, error)
	// Environment is the environment every program runs with.
	Environment []string
	// Run runs a prepared command to completion; nil runs it with
	// exec.Cmd.Run.
	Run func(command *exec.Cmd) error
}

// Opener opens URLs in the browser.
type Opener struct {
	options Options
}

// New returns an Opener. Missing lookups behave as if they found nothing.
func New(options Options) *Opener { return &Opener{options: options} }

// Open opens address in the browser and waits for the launcher to finish.
// Only http, https and the vscode schemes are opened. An address with no
// scheme is taken as https, unless it names a file or an executable.
func (opener *Opener) Open(address string) error {
	parsed, err := opener.check(address)
	if err != nil {
		return err
	}
	name, arguments, err := opener.launcher()
	if err != nil {
		return err
	}
	return opener.run(name, append(arguments, parsed.String()))
}

func (opener *Opener) run(name string, arguments []string) error {
	path, err := opener.lookPath(name)
	if err != nil {
		return err
	}
	command := exec.Command(path, arguments...)
	// A nil Env would inherit the whole environment, token variables included.
	command.Env = opener.options.Environment
	if command.Env == nil {
		command.Env = []string{}
	}
	// A nil Stdout and Stderr send the output to the null device.
	command.Stdout = nil
	command.Stderr = nil
	if opener.options.Run != nil {
		return opener.options.Run(command)
	}
	return command.Run()
}

func (opener *Opener) lookPath(name string) (string, error) {
	if opener.options.LookPath == nil {
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	return opener.options.LookPath(name)
}

func (opener *Opener) getEnvironment(name string) string {
	if opener.options.GetEnvironment == nil {
		return ""
	}
	return opener.options.GetEnvironment(name)
}

// configured returns the launcher the user set, in order of precedence.
func (opener *Opener) configured() string {
	if launcher := opener.getEnvironment("GH_BROWSER"); launcher != "" {
		return launcher
	}
	if opener.options.ConfiguredBrowser != nil {
		if launcher := opener.options.ConfiguredBrowser(); launcher != "" {
			return launcher
		}
	}
	return opener.getEnvironment("BROWSER")
}

// launcher returns the program and its leading arguments; the URL goes last.
func (opener *Opener) launcher() (string, []string, error) {
	if configured := opener.configured(); configured != "" {
		words, err := shlex.Split(configured)
		if err != nil {
			return "", nil, fmt.Errorf("parse browser launcher: %w", err)
		}
		if len(words) == 0 {
			return "", nil, errors.New("parse browser launcher: no program named")
		}
		return words[0], words[1:], nil
	}
	return opener.platformLauncher()
}

func (opener *Opener) platformLauncher() (string, []string, error) {
	switch opener.options.Platform {
	case "darwin":
		return "open", nil, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler"}, nil
	}
	providers := []string{"xdg-open", "x-www-browser", "www-browser", "wslview"}
	for _, provider := range providers {
		if _, err := opener.lookPath(provider); err == nil {
			return provider, nil, nil
		}
	}
	return "", nil, &exec.Error{Name: strings.Join(providers, ","), Err: exec.ErrNotFound}
}

func supportedScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "vscode", "vscode-insiders":
		return true
	}
	return false
}

// check rejects what must not be handed to a launcher and returns the URL to
// open.
func (opener *Opener) check(address string) (*url.URL, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("opening unparsable URL is unsupported: %s", address)
	}
	if supportedScheme(parsed.Scheme) {
		return parsed, nil
	}
	if parsed.Scheme != "" {
		return nil, fmt.Errorf("opening unsupported URL scheme: %s", address)
	}
	// A launcher may run a file or open it with an associated application.
	// Symlinks are not followed.
	if opener.options.Lstat != nil {
		if info, _ := opener.options.Lstat(address); info != nil {
			return nil, fmt.Errorf("opening files or directories is unsupported: %s", address)
		}
	}
	if opener.options.LookPath != nil {
		if path, _ := opener.options.LookPath(address); path != "" {
			return nil, fmt.Errorf("opening executables is unsupported: %s", address)
		}
	}
	parsed.Scheme = "https"
	return parsed, nil
}
