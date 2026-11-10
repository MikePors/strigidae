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
// finish ends this test, so it starts no second render beside a hung one
// (later tests in the package may, but only once this one has failed). A
// panic is recovered and reported, so it fails this test rather than ending
// the test binary.
func TestFencesOfBrokenLexersRender(test *testing.T) {
	type outcome struct {
		output    string
		recovered any
	}
	renderer := New(theme.New(true))
	renderer.Render("warm up", 60)
	for _, tag := range []string{"jsonata", "jungle", "yang", "YANG"} {
		for _, fence := range []string{"```", "~~~"} {
			for _, text := range []string{"/* a", "/* a /* b */"} {
				body := "text\n\n" + fence + tag + "\n" + text + "\n" + fence + "\n"
				rendered := make(chan outcome, 1)
				go func() {
					var result outcome
					defer func() {
						result.recovered = recover()
						rendered <- result
					}()
					result.output = renderer.Render(body, 60)
				}()
				select {
				case result := <-rendered:
					if result.recovered != nil {
						test.Errorf("%s %s fence holding %q: Render panicked: %v", fence, tag, text, result.recovered)
						continue
					}
					for _, word := range strings.Fields(text) {
						if !strings.Contains(result.output, word) {
							test.Errorf("%s %s fence: output %q does not hold %q of the fence text", fence, tag, result.output, word)
						}
					}
				case <-time.After(renderTimeout):
					test.Fatalf("%s %s fence holding %q: Render did not finish within %v", fence, tag, text, renderTimeout)
				}
			}
		}
	}
}
