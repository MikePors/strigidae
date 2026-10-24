package terminalpalette

import "golang.org/x/sys/unix"

const (
	getTermios = unix.TCGETS
	setTermios = unix.TCSETS
	// restoreTermios also discards unread input.
	restoreTermios = unix.TCSETSF
)
