# UI guide (`internal/userinterface/...`)

Rules for the app shell, components and views. The root `CLAUDE.md` still applies.

## Bubble Tea conventions

- **No I/O in `Update` or `View`.** Network calls, file I/O, and `$EDITOR` run inside a `tea.Cmd`; results come back as messages.
- **Result messages are typed and specific**: `pullRequestsLoadedMessage{repository, pullRequests, err}` rather than a generic message. Include enough identity (repo, PR number, a request sequence number) to discard stale responses that arrive after the user has moved on.
- **GitHub access** goes only through the `github.Client` interface and domain types; never import `internal/github/network`. API errors arrive typed (`errors.Is`/`errors.As`), and the view decides how to show them.
- **No goroutines** outside of `tea.Cmd`s, and no shared mutable state between the model and commands.
- **Only the root model is a `tea.Model`.** In Bubble Tea v2, `tea.Model.View()` returns a `tea.View` struct; the root wraps its output in one. Screens implement our own `userinterface.View` interface (`view.go`): `Update(msg) (View, tea.Cmd)` and `Render(width, height int) string`, plus title and help bindings.
- **Navigation is message-driven.** Views are composed by the root model's **view stack**. A view opens another by building it and returning a command producing `userinterface.PushMessage{View: view}` (or `userinterface.PopMessage`); it never reaches into the stack. `cmd/strig` builds the first view.
- **Message routing.** Keys go only to the top view. Every other message (command results, ticks) goes to every view in the stack, so a view must ignore results that aren't its own (see the identity rule above).
- **Loading and lifecycle.** A view that loads data implements `userinterface.Loader`; the status bar spinner follows the top view. A view revealed by `:q` gets `userinterface.Resumer.Resumed()` and may refresh.
- **Embedded bubbles components** (viewport, list, …): drive them through their methods (`ScrollDown`, `GotoTop`, …) in response to `keys.Message`; never forward a `keys.Message` to their `Update`.
- **Text inputs capture keys.** A view with a focused text input (filter, command line, reply box) implements `userinterface.KeyCapturer`; while it captures, it receives every key raw and no global bindings or sequences fire. `esc` must always release capture, and every capturing component needs a test proving it.
- **Scrolling lists.** Any view or component with a cursor in a scrolling list uses `internal/userinterface/components/scroll.Window` and never re-implements clamping or follow logic.
- **Sizing**: views receive the space they may draw in and must not draw outside it. Handle `tea.WindowSizeMsg` everywhere.
- **Untrusted text.** Pass all GitHub/git text through `internal/userinterface/terminaltext` before it reaches the screen: `Clean` for single-line text, `CleanBlock` before markdown rendering. Test with a hostile fixture (escape sequences, C1 controls, bidi overrides, invalid UTF-8). Accepted risk, don't re-report: markdown links render as OSC 8 hyperlinks (the URL is sanitised by `CleanStyled`), so link text can differ from its target, as on github.com.
- **Styling**: no inline colours or ad-hoc `lipgloss.NewStyle()` in views or components; add a named style to `internal/userinterface/theme`.
- **Performance**: large diffs must stay smooth. Render only the visible window, and cache expensive work (syntax highlighting, markdown rendering) keyed by content and width.

## Actions that write to GitHub

The maintainer's rules for when strig asks before acting:

- Anything that publishes or changes what other people see (submitting a review, merging, editing a published comment) always asks `y/N`, and always has a command (`:approve`, `:merge`). A key may also start it, but the `y/N` still follows; no single keypress ever publishes.
- Private drafts (pending review comments, the Files "viewed" mark) are added and edited without `y/N`.
- **Every deletion asks `y/N`**, draft or published.
- Cancelling happens only at the `y/N`. An empty body is an error, not a cancel, and nothing is written.
- Keys with a command behind them are welcome (`e` / `D` and `:edit-comment` / `:delete-comment`; `x` and `:resolve` / `:unresolve`).
- Re-read the state before asking and again after `y`; if it changed, ask again or refuse and refresh. Never write on a stale yes.

## Keybindings

Vim-first. All bindings are declared with `bubbles/key` and registered through `internal/userinterface/keys`, so the help overlay (`?`) is generated from one source of truth. Views never match on raw key strings.

**Notation.** Write keys exactly as the code does, because these strings are what the code matches on:
- control keys: `ctrl+d` (never `ctrl-d`, which silently never matches)
- shifted letters: the letter itself, `G`, `R`, `N` (not `shift+g`)
- shifted punctuation: the character itself, `}` (not `shift+]`). This relies on terminals reporting the key's text, which holds while strig requests only the kitty disambiguate flag; enabling more keyboard enhancements must keep `Text` populated
- special keys: `esc`, `enter`, `tab`, `backspace`, `up`, `down`
- **multi-key sequences are space-separated**: `g g`, `z a`, `g t`. `bubbles/key` only matches single keypresses, so sequences are matched by the sequence matcher in `internal/userinterface/keys` (pending prefix shown in the status bar, ~500ms timeout). Views just declare them in their bindings; they never track key history themselves.

The help overlay may display sequences compactly (`gg`, `za`).

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
| `/`, `n` / `N` | search the list (Files pane, PR lists), next / previous match; `/` opens the search prompt in the bottom line through `userinterface.Prompter` (the shell opens it in the same update as the key; the view hears `PromptResultMessage` on every change, paste included, on `enter` and on `esc`; currently every row stays and matching rows are marked) |
| `o` | open the current item in the browser |
| `R` | refresh |
| `c` | new comment (top-level, or on the cursor line / visual selection in a diff) |
| `r` | reply to the focused comment thread |
| `x` | resolve the comment thread at the cursor, or unresolve it if it is resolved (diff; asks `y/N`) |
| `:` | command line |
| `]` / `[` | next / previous chunk of what the pane shows: hunk in a diff, section in the Overview |
| `}` / `{` | next / previous file |
| `)` / `(` | next / previous comment thread (diff) |

There is deliberately **no single-key back or quit**: `q` and `ctrl+c` are unbound. `ctrl+o` / `ctrl+i` (back / forward through views) and `tab` are reserved for later; terminals send the same byte for `tab` and `ctrl+i`.

**Counts.** Digits `1`-`9` start a count and `0` continues it (`11j`, `3]`, `42G`), capped at 9999. The sequence matcher parses it and delivers it as `keys.Message.Count` (0 means none; `Repeat()` gives at least 1). A view applies it only to `j`/`k`, `]`/`[`, `}`/`{`, `)`/`(`, `ctrl+d`/`ctrl+u`/`ctrl+f`/`ctrl+b`, `z h`/`z l`, `n`/`N` (the count-th match), and `G` (with a count: go to line N, which in a diff is the new-side line number, or the nearest shown one; row N in lists); every other key ignores it and acts once. The count clears on any other key, `esc`, and the sequence timeout. While a view captures keys, digits are plain input.

Commands (`:` then `enter`):

| Command | Meaning |
|---|---|
| `:q`, `:quit` | close the current view; quits strig on the root view |
| `:qa`, `:qall`, `:quitall` | quit strig |
| `:set [no]<option>`, `:set <option>?` | change a yes/no option for this session (the config file is not written): `wrapscan` (`n`/`N` wrap past the last or first match), `hlsearch` (matches are marked) `wrap` (long diff lines continue on the next rows; off by default) `hideviewed` (viewed files leave the Files pane after a short fade; off by default) and `hidereviewed` (hunks marked reviewed leave the diff pane after a short fade, leaving a placeholder; off by default); `:set` alone shows every value, several options may share a line, an unknown option is an error that changes nothing; the options are declared in `internal/options` and are also config keys |
| `:theme <value>` | switch the colour theme: a bundled theme's name or a theme file, resolved like the config's `theme:` but with relative paths taken from the current directory; on failure the error shows and the theme stays |
| `:approve`, `:request-changes`, `:review` | PR workspace: submit a review verdict (`:review` opens the chooser) |
| `:checks` | PR workspace: list the head commit's checks (failing first); `enter` opens the selected check's page in the browser |
| `:merge [merge\|squash\|rebase]` | PR workspace: merge the pull request after a `y/N` confirmation; no argument uses the first method the repo allows (merge commit, squash, rebase) |
| `:copy-filename` | PR workspace: copy the selected file's path to the clipboard (OSC 52; tmux needs `set -g set-clipboard on`) |
| `:edit-comment`, `:delete-comment` | PR workspace: change your comment at the diff cursor (same as `e` / `D`): the last comment of the focused thread you may change, pending ones first; deleting always asks `y/N`, editing a published comment asks `y/N` after the editor closes |
| `:resolve`, `:unresolve` | PR workspace: resolve or unresolve the comment thread at the diff cursor (`x` does whichever applies); both ask `y/N` naming the file and line, and refuse with a notice when the thread is already in that state |

**View commands.** The table above is static. A view with commands of its own implements `userinterface.Commander` (`Command(name, args) (tea.Cmd, handled bool)`) and its names are added to `viewCommands` in `command.go`. The root consults only the top view, and only when the static table has no match, so a command never reaches two views the way a broadcast result would. A listed name the top view declines (or a top view that is not a Commander) gives "not available here: name"; any other name gives "not a command: name". List the view's commands in the table above.

Reserved prefixes:

| Prefix | Use |
|---|---|
| `g …` | goto: `g g` top, `g t` / `g T` next / previous tab |
| `z …` | folds (expand/collapse threads, files), horizontal scrolling (`z h` / `z l` a few columns, `z H` / `z L` half a screen, as in vim) and screen positioning: `z t` / `z z` / `z b` scroll the cursor line to the top / middle / bottom of the view without moving the cursor (lists with a cursor; they take no count, and the Overview doesn't have them yet) |

Adding a binding or command: make sure it doesn't collide with the tables above or another binding active in the same view, add it with help text, and **list every new or changed binding in the PR description** so the maintainer can approve it. Any binding an issue marks "TBD" is the maintainer's call; propose one in the PR and ask.

## Rendering tests

- **Golden files** via `internal/golden`: compare `Render(w, h)` output at a fixed size against `testdata/*.golden`. Regenerate with `make golden` (`UPDATE_GOLDEN=1 go test ./...`) and **read the `.golden` diff before committing**. Whole-screen goldens always strip ANSI codes (`x/ansi`). A test about styling (focus borders, selection, colours) asserts specific elements against strings produced by the theme's own styles (for example `th.PaneBorderFocus.Render("╭")`) and never stores escape codes in a golden, so a palette change cannot break it.
- **Deterministic rendering**: tests must not depend on the terminal. Use a fixed theme and fixed Glamour style (never auto-detected light/dark), fixed widths, and an injected clock for relative times.
- Cover each view's main states: loading, empty, error, populated, and narrow widths.

## Theming

Colour flows through three layers, each built only from the one below (details in the `internal/userinterface/theme` package doc):

1. **Palette**: the 24 Base24 slots plus a light/dark variant (`theme/palette.go`; built-ins in `theme/palettes.go`). The only place colour values exist. The slots are unexported; only `roles.go` reads them.
2. **Roles**: what strig means by a colour (`text`, `muted`, `accent`, `added`, diff tints, syntax tokens, …), derived from slots in `theme/roles.go`. A theme may override any role with an exact colour; overrides are used as written, and only derived roles go through the contrast guard (a failing override is logged).
3. **Styles**: the named Lip Gloss styles on `theme.Theme`, built from roles only. Markdown (Glamour) styles are generated from the same roles. Code has one Chroma style per theme (`Theme.Syntax`): generated from the syntax roles through one token-to-role mapping (`theme.ClassOf`), or a bundled Chroma style the theme names with `syntax:`. The diff pane and Markdown code blocks both draw from it, per token type.

What `theme/enforce_test.go` checks (over every non-test file in `internal/userinterface`, with the rules proven on snippets):

- no `#rgb`/`#rrggbb`/`#rrggbbaa` string literal, `lipgloss.Color`/`ANSIColor`, `ansi.IndexedColor`, or `color.RGBA`-style literal outside the palette files;
- no named colour constants (`lipgloss.Red`, `color.Black`, …);
- no palette slot (`Base0x`, `.Slot`, `.colors`) outside `roles.go` and the palette files.

Rules for you:

- A new UI element uses an existing role. If none fits, add a role with a default derivation from slots (and a line in the role table in `docs/themes.md`).
- Never add palette slots, ANSI indices or raw colours to styles, views or components.
- Caches of rendered output (markdown, highlighting) include `Theme.Identifier` (a fingerprint of the final roles and the syntax style), or hold theme-independent data (token types).
- Chroma's style registry is a global map read by Glamour without a lock. Register styles (`ChromaStyleName`) and run Glamour only on the UI goroutine, never in a `tea.Cmd`.
- Tests that render Markdown or register Chroma styles don't use `test.Parallel` (the registry is unlocked; see the line above).
- Bubbles widgets that ship default styles (`textinput`, `help`, `list`, …) get their styles set from the theme. Never keep `DefaultStyles()` or `DefaultDarkStyles()`; the AST check can't see those.
