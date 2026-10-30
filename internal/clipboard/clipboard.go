// Package clipboard hands text to the system clipboard tool (pbcopy, wl-copy,
// xclip or xsel). It complements the OSC 52 escape, which works over SSH but
// which some terminals and multiplexers drop. It is not a clipboard library:
// it starts the first tool it finds and pipes the text to it.
package clipboard

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ErrNoTool is returned by Copy when no clipboard tool is available.
var ErrNoTool = errors.New("no clipboard tool found")

// Copier copies text to the system clipboard.
type Copier interface {
	// Copy returns ErrNoTool when there is no tool to hand the text to.
	Copy(ctx context.Context, text string) error
}

// waitDelay is how long Copy waits for a killed or finished tool's I/O.
const waitDelay = time.Second

// Options are the things a System depends on, injected so tests never look
// for or run a real program.
type Options struct {
	// Platform is runtime.GOOS.
	Platform string
	// GetEnvironment looks up one environment variable (os.Getenv).
	GetEnvironment func(name string) string
	// LookPath finds a program on PATH (exec.LookPath).
	LookPath func(name string) (string, error)
	// Environment is the environment the tool runs with: the scrubbed one from
	// internal/environment.
	Environment []string
	// Run runs a prepared command to completion; nil runs it with
	// exec.Cmd.Run.
	Run func(command *exec.Cmd) error
}

// System is the Copier that uses the platform's clipboard tool.
type System struct {
	options Options
}

// New returns a System. A missing GetEnvironment or LookPath means no tool is
// ever found.
func New(options Options) *System { return &System{options: options} }

type tool struct {
	name      string
	arguments []string
}

// candidates are the tools to try, in order, for the platform and environment.
func (system *System) candidates() []tool {
	switch system.options.Platform {
	case "darwin":
		return []tool{{name: "pbcopy"}}
	case "linux":
		var tools []tool
		if system.options.GetEnvironment != nil && system.options.GetEnvironment("WAYLAND_DISPLAY") != "" {
			tools = append(tools, tool{name: "wl-copy"})
		}
		if system.options.GetEnvironment != nil && system.options.GetEnvironment("DISPLAY") != "" {
			tools = append(tools,
				tool{name: "xclip", arguments: []string{"-selection", "clipboard"}},
				tool{name: "xsel", arguments: []string{"--clipboard", "--input"}},
			)
		}
		return tools
	}
	return nil
}

// Copy pipes text to the first tool found. The context bounds the tool's run.
// Standard output and error are left nil (the null device) rather than
// io.Discard: xclip and wl-copy leave a background process holding the
// selection, which would keep a pipe open and make the wait hang.
func (system *System) Copy(ctx context.Context, text string) error {
	if system.options.LookPath == nil {
		return ErrNoTool
	}
	for _, candidate := range system.candidates() {
		path, err := system.options.LookPath(candidate.name)
		if err != nil {
			continue
		}
		command := exec.CommandContext(ctx, path, candidate.arguments...)
		command.Stdin = strings.NewReader(text)
		command.Env = system.options.Environment
		// A tool that forks a child keeping its stdin open must not hold Copy
		// past the context's deadline.
		command.WaitDelay = waitDelay
		run := system.options.Run
		if run == nil {
			run = (*exec.Cmd).Run
		}
		if err := run(command); err != nil {
			return &ToolError{Tool: candidate.name, Err: err}
		}
		return nil
	}
	return ErrNoTool
}

// ToolError is a clipboard tool that was found but failed. It never carries
// the copied text.
type ToolError struct {
	Tool string
	Err  error
}

// Error implements error.
func (toolError *ToolError) Error() string {
	return "run " + toolError.Tool + ": " + toolError.Err.Error()
}

// Unwrap returns the underlying error.
func (toolError *ToolError) Unwrap() error { return toolError.Err }
