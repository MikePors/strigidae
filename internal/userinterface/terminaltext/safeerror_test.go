package terminaltext

import (
	"errors"
	"fmt"
	"testing"
)

var errSentinel = errors.New("sentinel")

func TestSafeError(test *testing.T) {
	err := SafeError(fmt.Errorf("load\x1b]52;c;aGk=\x07 failed\u202e: %w", errSentinel))
	if got, want := err.Error(), "load ]52;c;aGk=  failed : sentinel"; got != want {
		test.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, errSentinel) {
		test.Error("errors.Is should see through the wrapper")
	}
}
