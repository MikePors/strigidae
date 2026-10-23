// Package markdown renders Markdown to styled terminal text with Glamour and
// caches the result by content and width.
package markdown

import (
	"strings"

	"charm.land/glamour/v2"

	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

type cacheKey struct {
	body            string
	width           int
	themeIdentifier string
}

// Renderer renders Markdown with a Glamour style built from a theme's roles.
// It is not safe for concurrent use; views call it from Update and Render
// only.
type Renderer struct {
	theme theme.Theme
	// Only the current width is kept: callers render at one width at a time,
	// so a window resize drops what was built for the old one.
	width            int
	terminalRenderer *glamour.TermRenderer
	cache            map[cacheKey]string
}

// New returns a Renderer styled by th. The style comes from the theme, never
// from terminal detection, so output is deterministic. Build a new Renderer
// when the theme changes.
func New(activeTheme theme.Theme) *Renderer {
	return &Renderer{theme: activeTheme, cache: map[cacheKey]string{}}
}

// Render returns body rendered and wrapped to width columns, without the
// blank lines Glamour puts around a document. If Glamour fails it returns the
// raw text so content is never lost.
func (renderer *Renderer) Render(body string, width int) string {
	width = max(width, 10)
	if width != renderer.width {
		renderer.width, renderer.terminalRenderer = width, nil
		clear(renderer.cache)
	}
	key := cacheKey{body, width, renderer.theme.Identifier}
	if text, ok := renderer.cache[key]; ok {
		return text
	}
	// Glamour decodes HTML character references (&#x1b;, &rlm;, ...) into real
	// control and bidi runes, after any input sanitising, so clean the output.
	text := terminaltext.CleanStyled(renderer.render(body, width))
	renderer.cache[key] = text
	return text
}

func (renderer *Renderer) render(body string, width int) string {
	if renderer.terminalRenderer == nil {
		terminalRenderer, err := glamour.NewTermRenderer(glamour.WithStyles(styleConfig(renderer.theme)), glamour.WithChromaFormatter("terminal16m"), glamour.WithWordWrap(width-4))
		if err != nil {
			return body
		}
		renderer.terminalRenderer = terminalRenderer
	}
	out, err := renderer.terminalRenderer.Render(body)
	if err != nil {
		return body
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}
