# Inline image rendering investigation (#36)

Decision: automatically load and display images on supported terminals.
Keep placeholders while loading, on failure, and on unsupported terminals.

This branch begins with the protocol prototype requested by the issue. The
production image loader and markdown integration are not implemented yet.

## Prototype

Run from this checkout in a real terminal:

```sh
go run ./cmd/imageprobe -protocol kitty   # Ghostty
go run ./cmd/imageprobe -protocol iterm2  # WezTerm
```

Press `r` to transmit the generated gradient after the screen appears. `j`/`k`
scroll, `h` covers/uncover the document, and `q` quits. Resize and press `r`
again. This is a diagnostic command, with its own keys; it is not a strig view.
It reads no image files and makes no network requests.

The Kitty path transmits a virtual placement and renders Unicode placeholder
cells through Bubble Tea. The six image rows carry explicit row diacritics;
columns inherit left to right. The iTerm2 path reserves blank cells and uses
`tea.Raw` with cursor save/restore to draw only when the entire image fits.
It deliberately does not paper over repaint ordering with a timer.

Automated checks verify sample PNG dimensions, transmission chunk size,
placeholder cell widths and clipping, and the iTerm2 visibility guard. They do
not establish that either terminal actually displays the image correctly.

## Findings and remaining design gate

- The pinned ANSI and Lip Gloss libraries count placeholder cells correctly
  and preserve the row diacritic when truncating a row.
- Bubble Tea 2.1.1 builds its screen using Ultraviolet cells. Graphics transfers
  must bypass markdown, sanitization and text layout. `tea.Raw` supplies that
  bypass, but `tea.View` has no documented post-frame graphics hook.
- Kitty placements should follow the text cells, including partial vertical
  clipping. This still needs a live Ghostty check.
- Do not infer WezTerm Unicode-placeholder support from general Kitty support.
  Its [tracking issue](https://github.com/wezterm/wezterm/issues/986) still lists
  placeholder support as unfinished. WezTerm documents the
  [iTerm2 image protocol](https://wezterm.org/imgcat.html).
- The iTerm2 prototype draws only after an explicit `r`, once the cell frame is
  stable. Scrolling, overlays and resize may erase the image or leave stale
  pixels. A production path needs deterministic frame ordering and erasure;
  a delayed command is not a synchronization guarantee. Record these results
  before deciding whether the renderer needs an extension.

Protocol reference: [Kitty Unicode placeholders](https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders).

## Implementation after the rendering gate

1. Parse Markdown image nodes and HTML `img` attributes without matching fenced
   code. Preserve URL/alt metadata independently of rendered lines. Sanitize alt
   text with `terminaltext`; give placeholders an actionable row target.
2. Add image fetching at the `github.Client` boundary, including the fake. Use
   the current auth host's allowed attachment hosts, HTTPS, explicit redirect
   validation and credential scoping. Do not retain signed redirect URLs in
   errors or logs. Bound time, bytes, dimensions and decoded pixel count.
3. Fetch/decode/scale in commands. Keep a bounded session cache keyed by source
   URL and target dimensions. Reject stale results after navigation or resize.
   Never perform network or decoding work in Update/View.
4. Detect protocol support at startup, provide a force-on/off configuration,
   and integrate image rows in description and comment layouts. `o`/`enter`
   opens the image URL. Cache graphics transfers separately from text rendering.
5. Add fake-network limit/auth/redirect tests, placeholder goldens, stale-result
   and navigation tests, help/config docs, then run `make check`.
