package markdown

import (
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestCacheStaysBoundedAcrossResizes(test *testing.T) {
	renderer := New(theme.New(true))
	for width := 20; width < 120; width++ {
		renderer.Render("# Title\n\ntext", width)
		renderer.Render("other body", width)
	}
	if len(renderer.cache) != 2 {
		test.Errorf("cache size = %d, want 2 (current width only)", len(renderer.cache))
	}
}
