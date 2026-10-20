# UI guide (`internal/ui/...`)

Rules for the app shell, components and views. The root `CLAUDE.md` still applies.

## Bubble Tea conventions

- **No I/O in `Update` or `View`.** Network calls, file I/O, and `$EDITOR` run inside a `tea.Cmd`; results come back as messages.
- **Result messages are typed and specific**: `prsLoadedMsg{repo, prs, err}` rather than a generic message. Include enough identity (repo, PR number, a request sequence number) to discard stale responses that arrive after the user has moved on.
- **GitHub access** goes only through the `github.Client` interface and domain types; never import `internal/github/api`. API errors arrive typed (`errors.Is`/`errors.As`), and the view decides how to show them.
- **No goroutines** outside of `tea.Cmd`s, and no shared mutable state between the model and commands.
- **Only the root model is a `tea.Model`.** In Bubble Tea v2, `tea.Model.View()` returns a `tea.View` struct; the root wraps its output in one. Screens implement our own `ui.View` interface (`view.go`): `Update(msg) (View, tea.Cmd)` and `Render(width, height int) string`, plus title and help bindings.
- **Navigation is message-driven.** Views are composed by the root model's **view stack**. A view opens another by building it and returning a command producing `ui.PushMsg{View: v}` (or `ui.PopMsg`); it never reaches into the stack. `cmd/strig` builds the first view.
- **Message routing.** Keys go only to the top view. Every other message (command results, ticks) goes to every view in the stack, so a view must ignore results that aren't its own (see the identity rule above).
- **Loading and lifecycle.** A view that loads data implements `ui.Loader`; the status bar spinner follows the top view. A view revealed by `:q` gets `ui.Resumer.Resumed()` and may refresh.
- **Embedded bubbles components** (viewport, list, …): drive them through their methods (`ScrollDown`, `GotoTop`, …) in response to `keys.Msg`; never forward a `keys.Msg` to their `Update`.
- **Text inputs capture keys.** A view with a focused text input (filter, command line, reply box) implements `ui.KeyCapturer`; while it captures, it receives every key raw and no global bindings or sequences fire. `esc` must always release capture, and every capturing component needs a test proving it.
- **Sizing**: views receive the space they may draw in and must not draw outside it. Handle `tea.WindowSizeMsg` everywhere.
- **Untrusted text.** Pass all GitHub/git text through `internal/ui/termtext` before it reaches the screen: `Clean` for single-line text, `CleanBlock` before markdown rendering. Test with a hostile fixture (escape sequences, C1 controls, bidi overrides, invalid UTF-8).
- **Styling**: no inline colours or ad-hoc `lipgloss.NewStyle()` in views or components; add a named style to `internal/ui/theme`.
- **Performance**: large diffs must stay smooth. Render only the visible window, and cache expensive work (syntax highlighting, markdown rendering) keyed by content and width.

## Keybindings

Vim-first. All bindings are declared with `bubbles/key` and registered through `internal/ui/keys`, so the help overlay (`?`) is generated from one source of truth. Views never match on raw key strings.

**Notation.** Write keys exactly as the code does, because these strings are what the code matches on:
- control keys: `ctrl+d` (never `ctrl-d`, which silently never matches)
- shifted letters: the letter itself, `G`, `R`, `N` (not `shift+g`)
- special keys: `esc`, `enter`, `tab`, `backspace`, `up`, `down`
- **multi-key sequences are space-separated**: `g g`, `] f`, `g t`. `bubbles/key` only matches single keypresses, so sequences are matched by the sequence matcher in `internal/ui/keys` (pending prefix shown in the status bar, ~500ms timeout). Views just declare them in their bindings; they never track key history themselves.

The help overlay may display sequences compactly (`gg`, `]f`).

Reserved globally:

| Key | Meaning |
|---|---|
| `j` / `k` | down / up |
| `h` / `l` | focus the pane to the left / right (tiled views such as the PR workspace) |
| `g g` / `G` | top / bottom |
| `ctrl+d` / `ctrl+u` | half page down / up |
| `ctrl+f` / `ctrl+b` | full page down / up |
| `enter` | open / select |
| `esc` | cancel or clear state inside the view (filter, selection, pending keys, popups). **Never navigates.** |
| `?` | help overlay |
| `/`, `n` / `N` | filter or search, next / previous match |
| `o` | open the current item in the browser |
| `R` | refresh |
| `c` | new comment (top-level, or on the cursor line in a diff) |
| `r` | reply to the focused comment thread |
| `:` | command line |

There is deliberately **no single-key back or quit**: `q` and `ctrl+c` are unbound. `ctrl+o` / `ctrl+i` (back / forward through views) and `tab` are reserved for later; terminals send the same byte for `tab` and `ctrl+i`.

Commands (`:` then `enter`):

| Command | Meaning |
|---|---|
| `:q`, `:quit` | close the current view; quits strig on the root view |
| `:qa`, `:qall`, `:quitall` | quit strig |

Reserved prefixes:

| Prefix | Use |
|---|---|
| `g …` | goto: `g g` top, `g t` / `g T` next / previous tab |
| `] x` / `[ x` | next / previous *x*: `f` file, `h` hunk, `c` comment thread, `s` section |
| `z …` | folds (expand/collapse threads, files) and horizontal scrolling: `z h` / `z l` a few columns, `z H` / `z L` half a screen (as in vim) |

Adding a binding or command: make sure it doesn't collide with the tables above or another binding active in the same view, add it with help text, and **list every new or changed binding in the PR description** so the maintainer can approve it. Any binding an issue marks "TBD" is the maintainer's call; propose one in the PR and ask.

## Rendering tests

- **Golden files** via `internal/golden`: compare `Render(w, h)` output at a fixed size against `testdata/*.golden`. Regenerate with `make golden` (`UPDATE_GOLDEN=1 go test ./...`) and **read the `.golden` diff before committing**. Strip ANSI codes (`x/ansi`) unless the test is about styling.
- **Deterministic rendering**: tests must not depend on the terminal. Use a fixed theme and fixed Glamour style (never auto-detected light/dark), fixed widths, and an injected clock for relative times.
- Cover each view's main states: loading, empty, error, populated, and narrow widths.
