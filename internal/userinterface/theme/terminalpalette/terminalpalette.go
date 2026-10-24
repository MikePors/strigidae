// Package terminalpalette builds a theme from the terminal's own colours, for
// `theme: terminal`. Before the screen starts, strig asks the terminal for its
// foreground (OSC 10), background (OSC 11) and 16 ANSI colours (OSC 4),
// followed by a primary device attributes query (DA1). Terminals answer in
// order, so the DA1 reply marks the end of the answers; one that ignores the
// colour queries still answers DA1, so strig doesn't wait out the timeout.
package terminalpalette

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// Timeout is how long Ask waits for the terminal's answers.
const Timeout = 200 * time.Millisecond

// maximumReply bounds how much Ask reads, in case the terminal (or something
// typed during the wait) floods the input.
const maximumReply = 64 << 10

const ansiCount = 16

// Terminal is the controlling terminal Ask talks to. Read returns 0 bytes and no error when
// nothing arrives within a short wait, so Ask can check its clock.
type Terminal interface {
	Read(buffer []byte) (int, error)
	Write(data []byte) (int, error)
}

// Reply is what the terminal answered. Colours it did not report are nil.
type Reply struct {
	Foreground color.Color
	Background color.Color
	ANSI       [ansiCount]color.Color
	// Finished is set when the DA1 answer arrived, so no more answers follow.
	Finished bool
	// insideTmux adds the passthrough hint when nothing came back.
	insideTmux bool
}

// Queries returns the bytes to write: the colour queries, then DA1. Inside
// tmux each is wrapped in a passthrough sequence so it reaches the outer
// terminal; tmux forwards these only with `allow-passthrough on`.
func Queries(insideTmux bool) []byte {
	queries := []string{"\x1b]10;?\x1b\\", "\x1b]11;?\x1b\\"}
	for i := range ansiCount {
		queries = append(queries, fmt.Sprintf("\x1b]4;%d;?\x1b\\", i))
	}
	queries = append(queries, "\x1b[c")
	var out strings.Builder
	for _, query := range queries {
		if insideTmux {
			query = "\x1bPtmux;" + strings.ReplaceAll(query, "\x1b", "\x1b\x1b") + "\x1b\\"
		}
		out.WriteString(query)
	}
	return []byte(out.String())
}

// Ask writes the queries to terminal and reads answers until the DA1 reply
// arrives or Timeout passes by now.
func Ask(terminal Terminal, insideTmux bool, now func() time.Time) (Reply, error) {
	if _, err := terminal.Write(Queries(insideTmux)); err != nil {
		return Reply{}, fmt.Errorf("write colour queries: %w", err)
	}
	deadline := now().Add(Timeout)
	var received []byte
	buffer := make([]byte, 1024)
	for {
		reply := Parse(received)
		reply.insideTmux = insideTmux
		if reply.Finished || !now().Before(deadline) || len(received) >= maximumReply {
			return reply, nil
		}
		count, err := terminal.Read(buffer)
		received = append(received, buffer[:count]...)
		if err != nil {
			return reply, fmt.Errorf("read colour replies: %w", err)
		}
	}
}

// Parse reads the answers in data. Anything that isn't an answer to one of
// the queries (including incomplete sequences) is skipped.
func Parse(data []byte) Reply {
	var reply Reply
	for len(data) > 0 {
		start := bytes.IndexByte(data, '\x1b')
		if start < 0 || start+1 >= len(data) {
			break
		}
		data = data[start:]
		switch data[1] {
		case ']':
			body, rest, ok := operatingSystemCommand(data[2:])
			if !ok {
				return reply // incomplete: the rest hasn't arrived yet
			}
			reply.record(body)
			data = rest
		case '[':
			if length, ok := deviceAttributes(data[2:]); ok {
				reply.Finished = true
				data = data[2+length:]
				continue
			}
			data = data[2:]
		default:
			data = data[1:]
		}
	}
	return reply
}

// operatingSystemCommand splits an OSC body (after "ESC ]") from what follows
// its terminator, BEL or ST (ESC \). Any other ESC ends a stray "ESC ]" (an
// alt+] typed during the wait) without a body, so the answer it starts is
// still read.
func operatingSystemCommand(data []byte) (body string, rest []byte, ok bool) {
	for i, value := range data {
		switch {
		case value == '\a':
			return string(data[:i]), data[i+1:], true
		case value == '\x1b' && i+1 < len(data) && data[i+1] == '\\':
			return string(data[:i]), data[i+2:], true
		case value == '\x1b' && i+1 < len(data):
			return "", data[i:], true
		}
	}
	return "", nil, false
}

// deviceAttributes reports the length of a DA1 answer ("? digits and ; c")
// at the start of data (after "ESC [").
func deviceAttributes(data []byte) (int, bool) {
	if len(data) == 0 || data[0] != '?' {
		return 0, false
	}
	for i := 1; i < len(data); i++ {
		switch {
		case data[i] == 'c':
			return i + 1, true
		case data[i] != ';' && (data[i] < '0' || data[i] > '9'):
			return 0, false
		}
	}
	return 0, false
}

// record stores the colour an OSC 4, 10 or 11 answer carries.
func (reply *Reply) record(body string) {
	fields := strings.Split(body, ";")
	switch {
	case len(fields) == 2 && fields[0] == "10":
		reply.Foreground = parseColour(fields[1])
	case len(fields) == 2 && fields[0] == "11":
		reply.Background = parseColour(fields[1])
	case len(fields) == 3 && fields[0] == "4":
		index, err := strconv.Atoi(fields[1])
		if err == nil && index >= 0 && index < ansiCount {
			reply.ANSI[index] = parseColour(fields[2])
		}
	}
}

// parseColour reads an X11 colour specification, "rgb:R/G/B" with one to four
// hex digits per component (some terminals send "rgba:R/G/B/A"), or nil.
func parseColour(specification string) color.Color {
	components, ok := strings.CutPrefix(specification, "rgb:")
	count := 3
	if !ok {
		components, ok = strings.CutPrefix(specification, "rgba:")
		count = 4
	}
	parts := strings.Split(components, "/")
	if !ok || len(parts) != count {
		return nil
	}
	var channels [3]uint64
	for i := range channels {
		part := parts[i]
		value, err := strconv.ParseUint(part, 16, 16)
		if err != nil || len(part) == 0 || len(part) > 4 {
			return nil
		}
		maximum := uint64(1)<<(4*len(part)) - 1
		channels[i] = (value*255 + maximum/2) / maximum
	}
	colour, err := theme.ParseHex(fmt.Sprintf("#%02x%02x%02x", channels[0], channels[1], channels[2]))
	if err != nil {
		return nil
	}
	return colour
}

// reported counts the colours the terminal answered, out of 18.
func (reply Reply) reported() int {
	count := 0
	for _, colour := range append([]color.Color{reply.Foreground, reply.Background}, reply.ANSI[:]...) {
		if colour != nil {
			count++
		}
	}
	return count
}

// Theme builds the theme from a complete reply. It is Chosen, so a terminal
// background report doesn't replace it, and FromTerminal, so it leaves the
// terminal's colours alone. A missing or partial reply is an error saying why.
func (reply Reply) Theme() (theme.Theme, error) {
	switch reported := reply.reported(); {
	case reported == 0 && !reply.Finished && reply.insideTmux:
		return theme.Theme{}, errors.New("no answer from the terminal (under tmux, set -g allow-passthrough on)")
	case reported == 0 && !reply.Finished:
		return theme.Theme{}, errors.New("no answer from the terminal")
	case reported == 0:
		return theme.Theme{}, errors.New("the terminal does not report its colours")
	case reported < ansiCount+2:
		return theme.Theme{}, fmt.Errorf("the terminal reported only %d of %d colours", reported, ansiCount+2)
	}
	palette, err := theme.FromANSI("terminal", reply.Background, reply.Foreground, reply.ANSI)
	if err != nil {
		return theme.Theme{}, err
	}
	built := theme.FromPalette(palette)
	built.Chosen = true
	built.FromTerminal = true
	return built, nil
}
