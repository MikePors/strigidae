package dashboard

import (
	"errors"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestHostileRefreshErrorIsNeutralised(test *testing.T) {
	view, client := loadedView(test)
	hostile := "boom\x1b[2J\x07" + string(rune(0x202e)) + "\u009b"
	client.SetError(fake.MethodSearchPullRequests, errors.New(hostile))
	settle(view, press(view, "R"))
	raw := view.Render(100, 16) // not stripped: styling escapes are expected, hostile ones are not
	if !strings.Contains(raw, "refresh failed: boom") {
		test.Fatalf("the failed refresh is not shown: %q", raw)
	}
	for _, bad := range []string{"\x1b[2J", "\x07", string(rune(0x202e)), "\u009b"} {
		if strings.Contains(raw, bad) {
			test.Errorf("render contains %q from the error", bad)
		}
	}
}
