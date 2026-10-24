//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package terminalpalette

import (
	"errors"
	"time"
)

// ControllingTerminal is the controlling terminal; this platform has no
// support for asking it.
type ControllingTerminal struct{}

// Open reports that asking the terminal is not supported here.
func Open() (*ControllingTerminal, error) {
	return nil, errors.New("asking the terminal for its colours is not supported on this platform")
}

// Read never reads.
func (*ControllingTerminal) Read([]byte, time.Duration) (int, error) { return 0, errors.ErrUnsupported }

// Write never writes.
func (*ControllingTerminal) Write([]byte) (int, error) { return 0, errors.ErrUnsupported }

// Close does nothing.
func (*ControllingTerminal) Close() error { return nil }
