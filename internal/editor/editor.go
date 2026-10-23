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
	command *exec.Cmd
	path    string
}

// New writes initial to a new temporary .md file and returns a session whose
// command opens it in $VISUAL, then $EDITOR, then vi. getenv looks up
// environment variables (os.Getenv in production).
func New(initial string, getEnvironment func(string) string) (*Session, error) {
	file, err := os.CreateTemp("", "strig-*.md")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %w", err)
	}
	path := file.Name()
	_, writeError := file.WriteString(initial)
	if err := errors.Join(writeError, file.Close()); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("write temporary file: %w", err)
	}
	return &Session{command: Command(path, getEnvironment), path: path}, nil
}

// Command is the editor command, ready to run.
func (session *Session) Command() *exec.Cmd { return session.command }

// Path is the temporary file.
func (session *Session) Path() string { return session.path }

// Result reads back what the user wrote and removes the file. It must be
// called once, after the editor exited (also when it failed, to clean up).
func (session *Session) Result() (string, error) {
	bytes, err := os.ReadFile(session.path)
	readError := os.Remove(session.path)
	if err != nil {
		return "", fmt.Errorf("read edited file: %w", err)
	}
	if readError != nil && !errors.Is(readError, os.ErrNotExist) {
		return string(bytes), fmt.Errorf("remove temporary file: %w", readError)
	}
	return string(bytes), nil
}

// Command returns the command that opens path in the user's editor. Like
// git, it hands the editor value to the shell with the path as an argument
// (sh -c '$EDITOR "$@"' strig path), so quoting and paths with spaces work.
func Command(path string, getEnvironment func(string) string) *exec.Cmd {
	editor := fallback
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(getEnvironment(name)); value != "" {
			editor = value
			break
		}
	}
	return exec.Command("sh", "-c", editor+` "$@"`, "strig", path)
}
