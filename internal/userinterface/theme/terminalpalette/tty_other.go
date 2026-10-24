//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package terminalpalette

import "errors"

// TTY is the controlling terminal; this platform has no support for asking it.
type TTY struct{}

// Open reports that asking the terminal is not supported here.
func Open() (*TTY, error) {
	return nil, errors.New("asking the terminal for its colours is not supported on this platform")
}

// Read never reads.
func (*TTY) Read([]byte) (int, error) { return 0, errors.ErrUnsupported }

// Write never writes.
func (*TTY) Write([]byte) (int, error) { return 0, errors.ErrUnsupported }

// Close does nothing.
func (*TTY) Close() error { return nil }
