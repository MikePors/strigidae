package terminalpalette

import (
	"testing"

	"golang.org/x/sys/unix"
)

// TestRestoreDiscardsUnreadInput checks the terminal is restored with the
// request that flushes input, so a reply still in flight is not typed into
// strig.
func TestRestoreDiscardsUnreadInput(test *testing.T) {
	if restoreTermios != unix.TCSETSF {
		test.Errorf("restoreTermios = %#x, want TCSETSF (%#x)", uint(restoreTermios), uint(unix.TCSETSF))
	}
}
