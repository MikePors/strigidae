package markdown

import (
	"strings"
	"testing"
	"time"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// renderTimeout bounds a Render that only a hanging lexer would not finish.
const renderTimeout = 10 * time.Second

// TestFencesOfBrokenLexersRender: fences tagged with a lexer that used to hang
// or panic render, with their text. Render runs on one other goroutine while
// nothing else renders or registers styles, which keeps to the rule that
// Glamour and the Chroma style registry stay on one goroutine at a time; the
// style is warmed on the test goroutine first. The first render that does not
// finish ends the test, so no second render starts beside a hung one.
func TestFencesOfBrokenLexersRender(test *testing.T) {
	renderer := New(theme.New(true))
	renderer.Render("warm up", 60)
	for _, tag := range []string{"jsonata", "jungle", "yang", "YANG"} {
		for _, fence := range []string{"```", "~~~"} {
			for _, text := range []string{"/* a", "/* a /* b */"} {
				body := "text\n\n" + fence + tag + "\n" + text + "\n" + fence + "\n"
				rendered := make(chan string, 1)
				go func() { rendered <- renderer.Render(body, 60) }()
				select {
				case output := <-rendered:
					for _, word := range strings.Fields(text) {
						if !strings.Contains(output, word) {
							test.Errorf("%s %s fence: output %q does not hold %q of the fence text", fence, tag, output, word)
						}
					}
				case <-time.After(renderTimeout):
					test.Fatalf("%s %s fence holding %q: Render did not finish within %v", fence, tag, text, renderTimeout)
				}
			}
		}
	}
}
