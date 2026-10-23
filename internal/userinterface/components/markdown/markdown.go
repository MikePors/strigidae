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
	body    string
	width   int
	themeID string
}

// Renderer renders Markdown with a Glamour style built from a theme's roles.
// It is not safe for concurrent use; views call it from Update and Render
// only.
type Renderer struct {
	th theme.Theme
	// Only the current width is kept: callers render at one width at a time,
	// so a window resize drops what was built for the old one.
	width int
	tr    *glamour.TermRenderer
	cache map[cacheKey]string
}

// New returns a Renderer styled by th. The style comes from the theme, never
// from terminal detection, so output is deterministic. Build a new Renderer
// when the theme changes.
func New(th theme.Theme) *Renderer {
	return &Renderer{th: th, cache: map[cacheKey]string{}}
}

// Render returns body rendered and wrapped to width columns, without the
// blank lines Glamour puts around a document. If Glamour fails it returns the
// raw text so content is never lost.
func (r *Renderer) Render(body string, width int) string {
	width = max(width, 10)
	if width != r.width {
		r.width, r.tr = width, nil
		clear(r.cache)
	}
	k := cacheKey{body, width, r.th.ID}
	if s, ok := r.cache[k]; ok {
		return s
	}
	// Glamour decodes HTML character references (&#x1b;, &rlm;, ...) into real
	// control and bidi runes, after any input sanitising, so clean the output.
	s := terminaltext.CleanStyled(r.render(body, width))
	r.cache[k] = s
	return s
}

func (r *Renderer) render(body string, width int) string {
	if r.tr == nil {
		tr, err := glamour.NewTermRenderer(glamour.WithStyles(styleConfig(r.th)), glamour.WithChromaFormatter("terminal16m"), glamour.WithWordWrap(width-4))
		if err != nil {
			return body
		}
		r.tr = tr
	}
	out, err := r.tr.Render(body)
	if err != nil {
		return body
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}
