package ui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/ui/theme"
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
// empty line does nothing. The static table is consulted first; then top, if
// it is a Commander and the name is in viewCommands. A name nobody handles
// yields an error message.
func parseCommand(line string, top View) tea.Cmd {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	name, args, _ := strings.Cut(line, " ")
	args = strings.TrimSpace(args)
	for _, c := range commandTable() {
		for _, n := range c.names {
			if n == name {
				return c.run(args)
			}
		}
	}
	if slices.Contains(viewCommands, name) {
		if c, ok := top.(Commander); ok {
			if cmd, handled := c.Command(name, args); handled {
				return cmd
			}
		}
		return Error(fmt.Errorf("not available here: %s", name))
	}
	return Error(fmt.Errorf("not a command: %s", name))
}

// viewCommands are the names that views may handle through Commander. The
// shell keeps the list so it can tell "a command this view lacks" from a
// typo. Add a name here when a view starts handling it.
var viewCommands = []string{"approve", "request-changes", "review"}

// commandLine is the ":" prompt drawn in the status bar row. It is the
// reference pattern for key-capturing input: while active it reports
// CapturesKeys, handles esc by releasing capture, and the root forwards it
// every key.
type commandLine struct {
	input  textinput.Model
	active bool
}

var _ KeyCapturer = (*commandLine)(nil)

func newCommandLine(th theme.Theme) commandLine {
	in := textinput.New()
	in.Prompt = ":"
	c := commandLine{input: in}
	c.setTheme(th)
	return c
}

// setTheme styles the input from the theme's command-line styles, so the
// typed text is readable on light and dark backgrounds alike.
func (c *commandLine) setTheme(th theme.Theme) {
	state := textinput.StyleState{
		Text:        th.CommandText,
		Prompt:      th.CommandPrompt,
		Placeholder: th.CommandHint,
		Suggestion:  th.CommandHint,
	}
	c.input.SetStyles(textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textinput.CursorStyle{Color: th.CommandText.GetForeground(), Shape: tea.CursorBlock, Blink: true},
	})
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
