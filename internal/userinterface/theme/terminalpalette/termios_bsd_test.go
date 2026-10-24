//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package terminalpalette

import (
	"testing"

	"golang.org/x/sys/unix"
)

// TestRestoreDiscardsUnreadInput checks the terminal is restored with the
// request that flushes input, so a reply still in flight is not typed into
// strig.
func TestRestoreDiscardsUnreadInput(test *testing.T) {
	if restoreTermios != unix.TIOCSETAF {
		test.Errorf("restoreTermios = %#x, want TIOCSETAF (%#x)", uint(restoreTermios), uint(unix.TIOCSETAF))
	}
}
