// Package markdown renders Markdown to styled terminal text with Glamour and
// caches the result by content and width.
package markdown

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"

	"github.com/MikePors/strigidae/internal/ui/termtext"
)

type cacheKey struct {
	body  string
	width int
}

// Renderer renders Markdown with a fixed Glamour style. It is not safe for
// concurrent use; views call it from Update and Render only.
type Renderer struct {
	style string
	// Only the current width is kept: callers render at one width at a time,
	// so a window resize drops what was built for the old one.
	width int
	tr    *glamour.TermRenderer
	cache map[cacheKey]string
}

// New returns a Renderer using Glamour's dark or light style. The style is
// chosen explicitly, never auto-detected, so output is deterministic.
func New(dark bool) *Renderer {
	style := styles.LightStyle
	if dark {
		style = styles.DarkStyle
	}
	return &Renderer{style: style, cache: map[cacheKey]string{}}
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
	k := cacheKey{body, width}
	if s, ok := r.cache[k]; ok {
		return s
	}
	// Glamour decodes HTML character references (&#x1b;, &rlm;, ...) into real
	// control and bidi runes, after any input sanitising, so clean the output.
	s := termtext.CleanStyled(r.render(body, width))
	r.cache[k] = s
	return s
}

func (r *Renderer) render(body string, width int) string {
	if r.tr == nil {
		tr, err := glamour.NewTermRenderer(glamour.WithStandardStyle(r.style), glamour.WithWordWrap(width-4))
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
