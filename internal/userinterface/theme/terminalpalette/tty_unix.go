//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package terminalpalette

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// TTY is the controlling terminal, opened for Ask. It reads through raw
// system calls rather than an os.File: the runtime poller would make Read
// block until input arrives, while the terminal's own read timeout (VTIME)
// lets Ask notice its deadline.
type TTY struct {
	descriptor int
	saved      unix.Termios
}

// Open opens the controlling terminal with echo and line buffering off, and
// reads that give up after a tenth of a second. Close restores it.
func Open() (*TTY, error) {
	descriptor, err := unix.Open("/dev/tty", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open terminal: %w", err)
	}
	saved, err := unix.IoctlGetTermios(descriptor, getTermios)
	if err != nil {
		_ = unix.Close(descriptor) // nothing was changed
		return nil, fmt.Errorf("read terminal settings: %w", err)
	}
	raw := *saved
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1
	if err := unix.IoctlSetTermios(descriptor, setTermios, &raw); err != nil {
		_ = unix.Close(descriptor) // nothing was changed
		return nil, fmt.Errorf("set terminal settings: %w", err)
	}
	return &TTY{descriptor: descriptor, saved: *saved}, nil
}

// Read reads what the terminal sent, returning 0 bytes when nothing arrived
// within a tenth of a second.
func (tty *TTY) Read(buffer []byte) (int, error) {
	for {
		count, err := unix.Read(tty.descriptor, buffer)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return max(count, 0), err
	}
}

// Write writes data to the terminal.
func (tty *TTY) Write(data []byte) (int, error) {
	written := 0
	for written < len(data) {
		count, err := unix.Write(tty.descriptor, data[written:])
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return written, err
		}
		written += count
	}
	return written, nil
}

// Close restores the terminal's settings and closes it.
func (tty *TTY) Close() error {
	restoreErr := unix.IoctlSetTermios(tty.descriptor, setTermios, &tty.saved)
	closeErr := unix.Close(tty.descriptor)
	return errors.Join(restoreErr, closeErr)
}
