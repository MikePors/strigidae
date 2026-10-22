// Package editor hands a piece of text to the user's editor through a
// temporary Markdown file. It builds the command but does not run it, so the
// caller can run it in a way that suspends a terminal UI (tea.ExecProcess).
package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// fallback is used when neither $VISUAL nor $EDITOR is set.
const fallback = "vi"

// Session is one editing round: a temporary file and the command to edit it.
type Session struct {
	cmd  *exec.Cmd
	path string
}

// New writes initial to a new temporary .md file and returns a session whose
// command opens it in $VISUAL, then $EDITOR, then vi. getenv looks up
// environment variables (os.Getenv in production).
func New(initial string, getenv func(string) string) (*Session, error) {
	f, err := os.CreateTemp("", "strig-*.md")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %w", err)
	}
	path := f.Name()
	_, werr := f.WriteString(initial)
	if err := errors.Join(werr, f.Close()); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("write temporary file: %w", err)
	}
	return &Session{cmd: Command(path, getenv), path: path}, nil
}

// Cmd is the editor command, ready to run.
func (s *Session) Cmd() *exec.Cmd { return s.cmd }

// Path is the temporary file.
func (s *Session) Path() string { return s.path }

// Result reads back what the user wrote and removes the file. It must be
// called once, after the editor exited (also when it failed, to clean up).
func (s *Session) Result() (string, error) {
	b, err := os.ReadFile(s.path)
	rerr := os.Remove(s.path)
	if err != nil {
		return "", fmt.Errorf("read edited file: %w", err)
	}
	if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		return string(b), fmt.Errorf("remove temporary file: %w", rerr)
	}
	return string(b), nil
}

// Command returns the command that opens path in the user's editor. Like
// git, it hands the editor value to the shell with the path as an argument
// (sh -c '$EDITOR "$@"' strig path), so quoting and paths with spaces work.
func Command(path string, getenv func(string) string) *exec.Cmd {
	editor := fallback
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			editor = v
			break
		}
	}
	return exec.Command("sh", "-c", editor+` "$@"`, "strig", path)
}
