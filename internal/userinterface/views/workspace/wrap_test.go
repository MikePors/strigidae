package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// TestWrapReachesDiffPane: with WithWrap, a diff line longer than the pane
// continues on a row marked ↪ instead of being cut off.
func TestWrapReachesDiffPane(test *testing.T) {
	for _, wrap := range []bool{false, true} {
		client := fake.New()
		client.SetPullRequest(testDetail())
		client.SetDiff(testRepository, 7, readDiff(test))
		harness := &harness{test: test, client: client}
		harness.view = New(client, testRepository, 7,
			WithClock(func() time.Time { return testNow }), WithTheme(theme.New(true)), WithWrap(wrap))
		harness.send(tea.WindowSizeMsg{Width: 100, Height: 40})
		harness.run(harness.view.Init())
		harness.press("j", "j", "l") // retry.go, whose comment line is wider than the pane
		if got := strings.Contains(harness.screen(), "↪"); got != wrap {
			test.Errorf("wrap %v: continuation marker shown = %v\n%s", wrap, got, harness.screen())
		}
		if harness.view.Wrap() != wrap {
			test.Errorf("Wrap() = %v, want %v", harness.view.Wrap(), wrap)
		}
	}
}
