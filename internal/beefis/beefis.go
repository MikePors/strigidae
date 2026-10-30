// Package beefis makes strig a beefis unit: it can ask whatever beefis routes
// its questions to (typically a Claude pane) about a line of a pull request,
// and hear the answers.
//
// strig does not speak beefis's socket protocol itself. It runs
// `beefis ctl unit MANIFEST` as a child process, which introduces strig with
// the manifest and relays JSON lines: events strig writes to its stdin go to
// beefis, and commands beefis sends strig come out of its stdout.
package beefis

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// SessionVariable is set by beefis in the environment of every program in a
// pane.
const SessionVariable = "BEEFIS_SESSION"

// InPane reports whether strig runs in a beefis pane.
func InPane(getEnvironment func(string) string) bool {
	return getEnvironment(SessionVariable) != ""
}

// Question is the ask event: a question about lines of a pull request.
// Repository is "owner/name"; Side is "LEFT" or "RIGHT", the side Line and
// StartLine are on. StartLine is zero for a single line.
type Question struct {
	RequestIdentifier string `json:"requestIdentifier"`
	Repository        string `json:"repository"`
	Number            int    `json:"number"`
	Head              string `json:"head"`
	Path              string `json:"path"`
	Side              string `json:"side"`
	Line              int    `json:"line"`
	StartLine         int    `json:"startLine,omitempty"`
	Excerpt           string `json:"excerpt,omitempty"`
	Question          string `json:"question"`
}

// Answer is the answer command: the reply to the question with the same
// RequestIdentifier, or why there is none.
type Answer struct {
	RequestIdentifier string `json:"requestIdentifier"`
	Text              string `json:"text"`
	Error             string `json:"error,omitempty"`
}

// Names of the event strig emits and the command it takes.
const (
	askEvent      = "ask"
	answerCommand = "answer"
)

// manifest declares the ask event and the answer command. The fields follow
// beefis's parameter descriptions; command fields are all text because a
// handler's send carries a map of text.
const manifest = `{
  "tool": "strig",
  "version": "1",
  "events": [{
    "name": "ask",
    "description": "a question about lines of a pull request",
    "fields": [
      {"name": "requestIdentifier", "type": {"is": "text"}, "required": true},
      {"name": "repository", "description": "owner/name", "type": {"is": "text"}, "required": true},
      {"name": "number", "type": {"is": "integer"}, "required": true},
      {"name": "head", "description": "the commit the lines are from", "type": {"is": "text"}, "required": true},
      {"name": "path", "type": {"is": "text"}, "required": true},
      {"name": "side", "type": {"is": "choice", "values": ["LEFT", "RIGHT"]}, "required": true},
      {"name": "line", "type": {"is": "integer"}, "required": true},
      {"name": "startLine", "type": {"is": "integer"}, "required": false},
      {"name": "excerpt", "description": "the diff lines asked about", "type": {"is": "text"}, "required": false},
      {"name": "question", "type": {"is": "text"}, "required": true}
    ]
  }],
  "commands": [{
    "name": "answer",
    "description": "the answer to an ask",
    "fields": [
      {"name": "requestIdentifier", "type": {"is": "text"}, "required": true},
      {"name": "text", "type": {"is": "text"}, "required": true},
      {"name": "error", "type": {"is": "text"}, "required": false}
    ]
  }]
}
`

// line is one JSON line in either direction.
type line struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

// Unit is strig's connection to beefis. Ask and Receive may run at the same
// time, from different goroutines.
type Unit struct {
	writeLock sync.Mutex
	writer    io.WriteCloser
	reader    *bufio.Reader
	// stop ends the child process and removes the manifest file; nil for a
	// unit made from plain streams.
	stop func() error
}

// Start runs `program ctl unit` with environment as its environment (in
// os.Environ form). What it writes to stderr goes to log.
func Start(program string, environment []string, log io.Writer) (*Unit, error) {
	manifestFile, err := os.CreateTemp("", "strig-beefis-*.json")
	if err != nil {
		return nil, fmt.Errorf("beefis manifest: %w", err)
	}
	_, writeErr := manifestFile.WriteString(manifest)
	if err := errors.Join(writeErr, manifestFile.Close()); err != nil {
		return nil, errors.Join(fmt.Errorf("beefis manifest: %w", err), os.Remove(manifestFile.Name()))
	}
	process := exec.Command(program, "ctl", "unit", manifestFile.Name())
	process.Env = environment
	process.Stderr = log
	stdin, stdinErr := process.StdinPipe()
	stdout, stdoutErr := process.StdoutPipe()
	if err := errors.Join(stdinErr, stdoutErr); err != nil {
		return nil, errors.Join(fmt.Errorf("start beefis unit: %w", err), os.Remove(manifestFile.Name()))
	}
	if err := process.Start(); err != nil {
		return nil, errors.Join(fmt.Errorf("start beefis unit: %w", err), os.Remove(manifestFile.Name()))
	}
	unit := New(stdin, stdout)
	unit.stop = func() error {
		// Closing stdin ends `beefis ctl unit`.
		closeErr := stdin.Close()
		return errors.Join(closeErr, process.Wait(), os.Remove(manifestFile.Name()))
	}
	return unit, nil
}

// New makes a unit that writes events to writer and reads commands from
// reader, for tests and for a transport other than a child process.
func New(writer io.WriteCloser, reader io.Reader) *Unit {
	return &Unit{writer: writer, reader: bufio.NewReader(reader)}
}

// Ask emits question.
func (unit *Unit) Ask(question Question) error {
	data, err := json.Marshal(question)
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}
	encoded, err := json.Marshal(line{Name: askEvent, Data: data})
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}
	unit.writeLock.Lock()
	defer unit.writeLock.Unlock()
	if _, err := unit.writer.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("ask: %w", err)
	}
	return nil
}

// Receive waits for the next answer. Lines that are not answers are skipped.
// It returns io.EOF once beefis has closed the connection.
func (unit *Unit) Receive() (Answer, error) {
	for {
		text, err := unit.reader.ReadString('\n')
		if strings.TrimSpace(text) != "" {
			var received line
			if decodeErr := json.Unmarshal([]byte(text), &received); decodeErr != nil {
				return Answer{}, fmt.Errorf("read beefis command: %w", decodeErr)
			}
			if received.Name == answerCommand {
				var answer Answer
				if decodeErr := json.Unmarshal(received.Data, &answer); decodeErr != nil {
					return Answer{}, fmt.Errorf("read beefis answer: %w", decodeErr)
				}
				return answer, nil
			}
		}
		if err != nil {
			return Answer{}, err
		}
	}
}

// Close ends the connection.
func (unit *Unit) Close() error {
	if unit.stop != nil {
		return unit.stop()
	}
	return unit.writer.Close()
}
