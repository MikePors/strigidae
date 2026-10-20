# CLAUDE.md

Guidance for anyone (human or agent) working on **strigidae**, a terminal UI for reviewing GitHub pull requests with vim keybindings. The binary is `strig`.

Read this whole file before starting an issue. If something here is wrong or missing, say so in your PR rather than silently working around it.

## Ground rules

- **Clean-room.** Design from the issue's requirements. Do not copy code, docs, keymaps, or config formats from other GitHub TUIs, and do not name other GitHub TUIs in code, comments, docs, issues, or PRs.
- **One issue → one branch → one PR.** Stay inside the issue's scope. Unrelated problems you notice go in the PR description under "Follow-ups", not in the diff.
- **Never** push to `main`, merge PRs, force-push a branch you didn't create, or close issues by hand. The maintainer merges.
- **`make check` must pass** before you push. It is exactly what CI runs.
- If the issue is ambiguous in a way that changes user-visible behaviour (keys, layout, wording, config format), stop and ask rather than guess.

## Stack

| Concern | Choice |
|---|---|
| Language | Go (version pinned in `go.mod`; the `go` command fetches the toolchain) |
| TUI framework | Bubble Tea v2 — `charm.land/bubbletea/v2` |
| Widgets | `charm.land/bubbles/v2` (key bindings, viewport, spinner, text input, …) |
| Styling | `charm.land/lipgloss/v2` |
| Markdown | `charm.land/glamour/v2` |
| Syntax highlighting | `github.com/alecthomas/chroma/v2` |
| GitHub auth + API | `github.com/cli/go-gh/v2` (reuses the `gh` CLI's login; GraphQL + REST clients) |
| Test diffs | `github.com/google/go-cmp/cmp` |

Use the `charm.land/...` v2 import paths. The v1 paths (`github.com/charmbracelet/{bubbletea,bubbles,lipgloss,glamour}`) are banned by the linter. Other `github.com/charmbracelet/x/...` helpers such as `x/ansi` are fine.

**Adding any other dependency** needs a one-line justification in the PR description. Prefer the standard library and the libraries above.

## Layout

```
cmd/strig/              main: flag parsing, wiring real dependencies, starting the program
internal/version/       build version
internal/github/        domain types (PullRequest, ReviewThread, Comment, …), the Client interface, typed errors
internal/github/api/    real Client implementation (go-gh GraphQL/REST); the only package that talks to the network
internal/github/fake/   in-memory Client for tests and demos
internal/diff/          unified-diff parsing and modelling (pure, no UI)
internal/config/        config file loading and validation
internal/editor/        $EDITOR handoff for writing comments
internal/gitrepo/       infer the GitHub repo (host + owner/name) from a checkout's remotes
internal/logging/       log file setup
internal/golden/        golden-file test helper
internal/ui/            app shell: root model, the View interface, navigation and status messages, status bar, command line
internal/ui/keys/       keymap registry and multi-key sequence handling
internal/ui/theme/      Lip Gloss styles; every colour lives here
internal/ui/components/ reusable widgets (PR table, markdown pane, thread, …)
internal/ui/views/<v>/  one package per screen: prlist, prdetail, diff, dashboard, …
```

Packages that don't exist yet are created by the issue that first needs them.

Dependencies point one way:

```
cmd/strig ──► ui/views/* ──► ui/components ──► ui ──► ui/keys, ui/theme
    │              │
    │              └──────► github, diff, config, editor   (non-UI)
    └──► github/api  (the only importer of the real client)
```

- `internal/ui` (the shell) and `ui/keys`, `ui/theme`, `ui/components` **never import `ui/views/*`**. A view opens another view by **building it and sending `ui.PushMsg{View: v}`**; the root model only manages the stack. `cmd/strig` builds the first view.
- Views may import other views (e.g. `prlist` builds a `prdetail`), as long as there's no cycle.
- Non-UI packages (`github`, `diff`, `config`, `editor`, `gitrepo`, `logging`, `version`) never import UI packages.
- Only `cmd/strig` imports `internal/github/api`.

These rules are enforced by `depguard` in `.golangci.yml`; `make lint` fails on a violation.

## Architecture

### The GitHub boundary

- The UI depends only on the `github.Client` **interface** and the domain types in `internal/github`. GraphQL response structs stay private to `internal/github/api` and are mapped to domain types there.
- Every `Client` method takes a `context.Context` first.
- Prefer GraphQL; review threads, resolving, and pending reviews only exist properly there. Use REST only where GraphQL lacks something (e.g. raw diffs).
- Never hardcode `github.com` as the host; take it from the auth/config so GitHub Enterprise can work later.
- API failures come back as typed errors (e.g. `github.ErrNotFound`, `*github.RateLimitError`) wrapped with `%w`. The UI decides how to display them.

### Bubble Tea conventions

- **No I/O in `Update` or `View`.** Network calls, file I/O, and `$EDITOR` run inside a `tea.Cmd`; results come back as messages.
- **Result messages are typed and specific**: `prsLoadedMsg{repo, prs, err}` rather than a generic message. Include enough identity (repo, PR number, a request sequence number) to discard stale responses that arrive after the user has moved on.
- **No goroutines** outside of `tea.Cmd`s, and no shared mutable state between the model and commands.
- **Only the root model is a `tea.Model`.** In Bubble Tea v2, `tea.Model.View()` returns a `tea.View` struct; the root wraps its output in one. Screens implement our own `ui.View` interface (`internal/ui/view.go`): `Update(msg) (View, tea.Cmd)` and `Render(width, height int) string`, plus title and help bindings.
- **Views** live in `internal/ui/views/<name>` and are composed by the root model's **view stack**. Navigation is message-driven: a view returns a command producing `ui.PushMsg` / `ui.PopMsg`; it never reaches into the stack itself.
- **Message routing.** Keys go only to the top view. Every other message (command results, ticks) goes to every view in the stack, so a view must ignore results that aren't its own (see the identity rule above).
- **Loading and lifecycle.** A view that loads data implements `ui.Loader`; the status bar spinner follows the top view. A view revealed by `:q` gets `ui.Resumer.Resumed()` and may refresh.
- **Embedded bubbles components** (viewport, list, …): drive them through their methods (`ScrollDown`, `GotoTop`, …) in response to `keys.Msg`; never forward a `keys.Msg` to their `Update`.
- **Text inputs capture keys.** A view with a focused text input (filter, command line, reply box) implements `ui.KeyCapturer`; while it captures, it receives every key raw and no global bindings or sequences fire. `esc` must always release capture, and every capturing component needs a test proving it.
- **Sizing**: views receive the space they may draw in and must not draw outside it. Handle `tea.WindowSizeMsg` everywhere.
- **Styling**: no inline colours or ad-hoc `lipgloss.NewStyle()` in views; add a named style to `internal/ui/theme`.
- **Performance**: large diffs must stay smooth. Render only the visible window, and cache expensive work (syntax highlighting, markdown rendering) keyed by content and width.

### Logging

- `log/slog` to a file under `$XDG_STATE_HOME/strig/` (fallback `~/.local/state/strig/`); `strig --log-path` prints it. **Never** write to stdout or stderr while the TUI is running; it corrupts the screen.
- At startup the standard library's default loggers (`log`, `slog`) are redirected to the log file, so a dependency that logs can't corrupt the screen. That's one of the two allowed global mutations besides build-time values.
- The other: at startup `cmd/strig` sets `github.com/cli/browser`'s `Stdout` and `Stderr` to `io.Discard`, so browser launchers such as `xdg-open` (used by `o`) can't write to the screen.
- Never log tokens, auth headers, or full API response bodies.

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

There is deliberately **no single-key back or quit**: `q` and `ctrl+c` are unbound. `ctrl+o` / `ctrl+i` (back / forward through views) and `tab` are reserved for later; note that terminals send the same byte for `tab` and `ctrl+i`.

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

## Testing

- `go test ./...` must never touch the network or the real `gh` config. Use `internal/github/fake` or fixtures.
- **Table-driven tests** with the standard `testing` package; compare plain data structs with `cmp.Diff`. Models have unexported fields, which make `cmp.Diff` panic: test models through their behaviour (send messages, inspect returned commands and `Render` output) or exported accessors rather than comparing them whole.
- **Fixtures** (recorded GraphQL JSON, sample diffs) live in a `testdata/` directory next to the test. Scrub tokens and personal data from anything recorded.
- **View rendering**: golden-file tests via `internal/golden`. Compare `Render(w, h)` output at a fixed size against `testdata/*.golden`. Regenerate with `make golden` (`UPDATE_GOLDEN=1 go test ./...`) and **read the `.golden` diff before committing**. Strip ANSI codes (`x/ansi`) unless the test is about styling.
- **Deterministic rendering**: tests must not depend on the terminal. Use a fixed theme and fixed Glamour style (never auto-detected light/dark), fixed widths, and an injected clock for relative times.
- What must be tested: parsing and mapping logic, keymap/sequence handling, state transitions in `Update` (send a message, assert the new state and the returned command), and rendering of each view's main states (loading, empty, error, populated).
- No `time.Sleep` in tests. Inject clocks and timers.
- **Live GitHub is the maintainer's job.** Agents never post to real GitHub, so acceptance criteria like "works on a strigidae PR" or "reflected on github.com" are verified by the maintainer during review. In the PR, show the behaviour against `internal/github/fake` and say what the maintainer should check live.

## Code style

- `make fmt` (gofumpt + goimports) and `make lint` (golangci-lint) are authoritative. A `//nolint` must name the linter and explain why (`//nolint:errcheck // best-effort write to stderr`); the linter enforces this.
- Handle errors from writes. `_, _ = fmt.Fprintln(stderr, ...)` is acceptable only for best-effort output to stderr.
- Error strings are lower-case with no trailing punctuation. Wrap with context: `fmt.Errorf("load pull request %d: %w", n, err)`.
- Doc comments on every exported identifier and every package. Otherwise comment *why*, not *what*; keep comments sparse.
- Keep functions small and names plain. Avoid `utils`/`helpers`/`common` packages.
- No global mutable state apart from build-time values such as `version.Version`.

## Workflow

1. **Branch** off an up-to-date `main`: `<type>/<issue#>-<short-slug>`, e.g. `feat/6-pr-list`, `fix/31-diff-scroll`. Types: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`.
2. **Commit** in small, coherent steps. Commit messages and PR titles use [Conventional Commits](https://www.conventionalcommits.org/): `feat(diff): add hunk navigation`, `fix(api): handle rate limit reset`. The scope is optional and is usually the area or package.
3. **Run `make check`.** It runs fmt-check, lint, race tests, tidy-check, govulncheck, actionlint, and build, exactly the steps CI runs. (Install `shellcheck` locally for full parity: actionlint uses it to check `run:` scripts when it's on `PATH`, and CI has it.)
4. **Open a PR** against `main` using the PR template. It must include:
   - `Closes #N`
   - what changed and why
   - **how to try it** (the exact `strig` invocation and keys) and test notes
   - a screenshot or recording for visible UI changes
   - new keybindings and new dependencies, if any
   - follow-ups you noticed but left out of scope
5. CI must be green. Address review comments with new commits; don't rewrite history on a PR that's under review.

### Definition of done

- [ ] Acceptance criteria in the issue are met
- [ ] Tests cover the new logic and the main view states
- [ ] `make check` passes
- [ ] New keybindings appear in the help overlay
- [ ] User-facing changes are reflected in README/docs
- [ ] PR description complete (see above)

### Tooling notes

- Dev tools (golangci-lint, govulncheck, actionlint) are pinned in their own modules under `tools/<name>/` and run through the Makefile. Don't add tools to the main `go.mod`.
- After changing dependencies run `make tidy`.
- **Dev tools are not updated by Dependabot** (it doesn't understand the `tool` directive). A monthly workflow (`.github/workflows/update-tools.yml`) runs `make update-tools` and opens a PR.
- **Go toolchain vulnerabilities:** if `govulncheck` fails on a **standard library** vulnerability (the finding names a `go1.x` stdlib package and is fixed in a newer Go release), it is not your PR's problem. Don't change `go.mod`'s `go`/`toolchain` lines. Stop and report it; the maintainer bumps the toolchain in a separate `chore` PR and you merge `main` afterwards. Vulnerabilities in third-party modules are in scope only if your PR added or upgraded that module.
- `gh pr edit` fails with "Projects (classic) is being deprecated" on older `gh` versions. Update a PR body with `gh api -X PATCH repos/MikePors/strigidae/pulls/<N> -F body=@file.md` instead.
