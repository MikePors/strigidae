//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package terminalpalette

import "golang.org/x/sys/unix"

const (
	getTermios = unix.TIOCGETA
	setTermios = unix.TIOCSETA
	// restoreTermios also discards unread input.
	restoreTermios = unix.TIOCSETAF
)
