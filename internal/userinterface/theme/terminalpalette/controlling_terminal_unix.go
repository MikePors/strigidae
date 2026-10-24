//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package terminalpalette

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// ControllingTerminal is the controlling terminal, opened for Ask. It reads
// through raw system calls rather than an os.File: the runtime poller would
// make Read block until input arrives, while the terminal's own read timeout
// (VTIME) lets Ask notice its deadline.
type ControllingTerminal struct {
	descriptor int
	saved      unix.Termios
}

// Open opens the controlling terminal with echo, line buffering and signal
// keys off, and reads that give up after a tenth of a second. Close restores
// it. Signal keys are off so that ctrl+c during the wait can't kill strig
// before Close has restored the settings.
func Open() (*ControllingTerminal, error) {
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
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1
	if err := unix.IoctlSetTermios(descriptor, setTermios, &raw); err != nil {
		_ = unix.Close(descriptor) // nothing was changed
		return nil, fmt.Errorf("set terminal settings: %w", err)
	}
	return &ControllingTerminal{descriptor: descriptor, saved: *saved}, nil
}

// Read reads what the terminal sent, returning 0 bytes when nothing arrived
// within a tenth of a second.
func (terminal *ControllingTerminal) Read(buffer []byte) (int, error) {
	for {
		count, err := unix.Read(terminal.descriptor, buffer)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return max(count, 0), err
	}
}

// Write writes data to the terminal.
func (terminal *ControllingTerminal) Write(data []byte) (int, error) {
	written := 0
	for written < len(data) {
		count, err := unix.Write(terminal.descriptor, data[written:])
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
func (terminal *ControllingTerminal) Close() error {
	restoreErr := unix.IoctlSetTermios(terminal.descriptor, setTermios, &terminal.saved)
	closeErr := unix.Close(terminal.descriptor)
	return errors.Join(restoreErr, closeErr)
}
