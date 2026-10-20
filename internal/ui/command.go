package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// commandSpec is one entry in the command registry.
type commandSpec struct {
	names []string // the command and its aliases, without the leading ":"
	run   func(args string) tea.Cmd
}

// commandTable lists the commands the command line understands. Add new
// commands here; parsing and dispatch need no other change.
func commandTable() []commandSpec {
	return []commandSpec{
		{names: []string{"q", "quit"}, run: func(string) tea.Cmd { return Pop() }},
		{names: []string{"qa", "qall", "quitall"}, run: func(string) tea.Cmd { return Quit() }},
	}
}

// parseCommand turns a command line (without the ":") into a command. An
// empty line does nothing; an unknown name yields an error message.
func parseCommand(line string) tea.Cmd {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	name, args, _ := strings.Cut(line, " ")
	for _, c := range commandTable() {
		for _, n := range c.names {
			if n == name {
				return c.run(strings.TrimSpace(args))
			}
		}
	}
	return Error(fmt.Errorf("not a command: %s", name))
}

// commandLine is the ":" prompt drawn in the status bar row. It is the
// reference pattern for key-capturing input: while active it reports
// CapturesKeys, handles esc by releasing capture, and the root forwards it
// every key.
type commandLine struct {
	input  textinput.Model
	active bool
}

var _ KeyCapturer = (*commandLine)(nil)

func newCommandLine() commandLine {
	in := textinput.New()
	in.Prompt = ":"
	return commandLine{input: in}
}

// CapturesKeys implements KeyCapturer.
func (c *commandLine) CapturesKeys() bool { return c.active }

func (c *commandLine) open() tea.Cmd {
	c.input.Reset()
	c.active = true
	return c.input.Focus()
}

func (c *commandLine) close() {
	c.active = false
	c.input.Blur()
	c.input.Reset()
}

// handleKey processes one key press. esc cancels, enter submits, and
// backspace on an empty line cancels, as in vim. When the line is submitted
// the text is returned with ok true.
func (c *commandLine) handleKey(msg tea.KeyPressMsg) (line string, ok bool, cmd tea.Cmd) {
	switch msg.String() {
	case "esc":
		c.close()
	case "enter":
		line = c.input.Value()
		c.close()
		return line, true, nil
	case "backspace":
		if c.input.Value() == "" {
			c.close()
			return "", false, nil
		}
		fallthrough
	default:
		c.input, cmd = c.input.Update(msg)
	}
	return "", false, cmd
}

// update forwards non-key messages (cursor blink) to the input.
func (c *commandLine) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}
