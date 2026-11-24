package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func TestRelativeNumberOption(test *testing.T) {
	inliner := newInliner(test)
	// The cursor row: the first hunk header, under the gap above it.
	firstDiffLine := func() string {
		width, height := inliner.diffView().Size()
		for _, line := range strings.Split(inliner.diffView().Render(width, height), "\n") {
			if line = ansi.Strip(line); strings.HasPrefix(line, ">") {
				return line
			}
		}
		return ""
	}
	if line := firstDiffLine(); !strings.HasPrefix(line, ">@@") {
		test.Fatalf("setup: the cursor should be on the first hunk header, got %q", line)
	}
	inliner.send(userinterface.OptionsMessage{Values: options.Values{options.RelativeNumber: true}})
	if line := firstDiffLine(); !strings.HasPrefix(line, ">   0 @@") {
		test.Errorf("relativenumber should number the cursor row 0, got %q", line)
	}
	inliner.send(userinterface.OptionsMessage{Values: options.Values{options.RelativeNumber: false}})
	if line := firstDiffLine(); !strings.HasPrefix(line, ">@@") {
		test.Errorf("norelativenumber should hide the column, got %q", line)
	}
}
