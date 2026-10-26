package workspace

import (
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func TestHideResolvedOption(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	harness.selectFile()
	const resolvedHeader = "▸ carol 1 comment · 1d ago [resolved]"
	if !strings.Contains(harness.screen(), resolvedHeader) {
		test.Fatalf("resolved thread should show by default:\n%s", harness.screen())
	}

	harness.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: true}})
	if out := harness.screen(); strings.Contains(out, "[resolved]") || !strings.Contains(out, "▾ bob 2 comments") {
		test.Errorf("hideresolved should leave out only the resolved thread:\n%s", out)
	}
	harness.press(")")
	if strings.Contains(harness.cursorRow(), "carol") {
		test.Errorf(") should skip the hidden thread, landed on %q", harness.cursorRow())
	}

	harness.send(userinterface.OptionsMessage{Values: options.Values{options.HideResolved: false}})
	if !strings.Contains(harness.screen(), resolvedHeader) {
		test.Errorf("nohideresolved should show the resolved thread again:\n%s", harness.screen())
	}
}
