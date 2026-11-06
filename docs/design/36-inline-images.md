# Inline image rendering investigation (#36)

Maintainer decision (2026-11-06, implementation conversation): when asked
whether supported terminals should display images automatically or show
placeholders until expanded, the maintainer answered "Display images
automatically". Automatically load and display images on supported terminals.
Keep placeholders while loading, on failure, and on unsupported terminals.

This branch begins with the protocol prototype requested by the issue. The
production image loader and markdown integration are not implemented yet.
This temporary diagnostic command is due for removal once #36 lands. Its PR
references #36 rather than closing the still-incomplete feature issue.

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
Before the first `r`, Kitty placeholder cells already exist without an uploaded
image; an empty area or placeholder glyphs at that point is expected.

The Kitty path transmits a virtual placement and renders Unicode placeholder
cells through Bubble Tea. The six image rows carry explicit row diacritics;
columns inherit left to right. The iTerm2 path reserves blank cells and uses
`tea.Raw` with cursor save/restore to draw only when the entire image fits.
It deliberately does not paper over repaint ordering with a timer.

Automated checks verify sample PNG dimensions, transmission chunk size,
placeholder cell widths and clipping, exact protocol bytes (placement, cursor
coordinates, image identifier and row diacritics), and the iTerm2 visibility
guard at the top, bottom and minimum-width boundaries. They do
not establish that either terminal actually displays the image correctly.

## Findings and remaining design gate

- The pinned ANSI and Lip Gloss libraries count placeholder cells correctly
  and preserve the row diacritic when truncating a row.
- Bubble Tea 2.1.1 builds its screen using Ultraviolet cells. Graphics transfers
  must bypass markdown, sanitization and text layout. `tea.Raw` supplies that
  bypass, but `tea.View` has no documented post-frame graphics hook.
- Kitty placements should follow the text cells, including partial vertical
  clipping. This still needs a live Ghostty check.
- The foreground's RGB bytes encode the image identifier, not a theme colour.
  Bubble Tea downsamples colours to the detected profile. ANSI256 conversion
  changes that identifier, so this prototype requires truecolor end to end.
  Test directly in the terminal first; tmux also needs APC passthrough, which
  this probe does not wrap. Record TERM, COLORTERM and whether tmux is involved.
- Kitty `q=2` suppresses error replies as well as success replies. A blank image
  therefore cannot distinguish rejected transmission from failed placeholder
  rendering. A future diagnostic extension should use `q=1` and display parsed,
  sanitized error replies; changing the flag alone would leave errors unhandled.
- The quit-time Kitty delete is best effort. `tea.Raw` queues it and shutdown
  may leave the alternate screen before flushing it. Signals and errors bypass
  that command. A clean-looking shell does not prove the alternate-screen image
  was freed. Verify cleanup separately before using this lifecycle in production.
- Do not infer WezTerm Unicode-placeholder support from general Kitty support.
  Its [tracking issue](https://github.com/wezterm/wezterm/issues/986) still lists
  placeholder support as unfinished. WezTerm documents the
  [iTerm2 image protocol](https://wezterm.org/imgcat.html).
- The iTerm2 prototype draws only after an explicit `r`, once the cell frame is
  stable. Scrolling, overlays and resize may erase the image or leave stale
  pixels. A production path needs deterministic frame ordering and erasure;
  a delayed command is not a synchronization guarantee. Record these results
  before deciding whether the renderer needs an extension.

Live evidence is still pending: attach screenshots or recordings from both
target terminals, with terminal versions and observations for scrolling,
overlays, resize and exit. The WezTerm tracker is documentary evidence only;
run the Kitty probe there too to check the installed version's placeholder
behavior. Do not treat the automated tests as terminal-rendering evidence.

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
