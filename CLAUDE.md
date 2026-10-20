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

Use the `charm.land/...` v2 import paths, never the older `github.com/charmbracelet/...` v1 paths.

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
internal/ui/            app shell: root model, view stack, navigation messages, status bar
internal/ui/keys/       keymap registry and multi-key sequence handling
internal/ui/theme/      Lip Gloss styles; every colour lives here
internal/ui/components/ reusable widgets (PR table, markdown pane, thread, …)
internal/ui/views/<v>/  one package per screen: prlist, prdetail, diff, dashboard, …
```

Packages that don't exist yet are created by the issue that first needs them. Dependencies point one way: `views → components → ui/keys, ui/theme`; anything UI may import `github`, `diff`, `config`; `github`, `diff`, and `config` never import UI packages; only `cmd/strig` imports `github/api`.

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
- **Views** live in `internal/ui/views/<name>`, are their own `tea.Model`-style types, and are composed by the root model's **view stack**. Navigation is message-driven: a view emits a push/pop message (defined in `internal/ui`); it never reaches into the stack itself.
- **Sizing**: views receive the space they may draw in and must not draw outside it. Handle `tea.WindowSizeMsg` everywhere.
- **Styling**: no inline colours or ad-hoc `lipgloss.NewStyle()` in views; add a named style to `internal/ui/theme`.
- **Performance**: large diffs must stay smooth. Render only the visible window, and cache expensive work (syntax highlighting, markdown rendering) keyed by content and width.

### Logging

- `log/slog` to a file under `$XDG_STATE_HOME/strig/` (fallback `~/.local/state/strig/`). **Never** write to stdout or stderr while the TUI is running; it corrupts the screen.
- Never log tokens, auth headers, or full API response bodies.

## Keybindings

Vim-first. All bindings are declared in `internal/ui/keys` (using `bubbles/key`) so the help overlay (`?`) is generated from one source of truth. Views never match on raw key strings.

Reserved globally:

| Key | Meaning |
|---|---|
| `j` / `k` | down / up |
| `gg` / `G` | top / bottom |
| `ctrl-d` / `ctrl-u` | half page down / up |
| `ctrl-f` / `ctrl-b` | full page down / up |
| `enter` | open / select |
| `q` | back (quit on the root view) |
| `esc` | cancel / clear |
| `?` | help overlay |
| `/`, `n` / `N` | filter or search, next / previous match |
| `o` | open the current item in the browser |
| `R` | refresh |
| `c` | new comment (top-level, or on the cursor line in a diff) |
| `r` | reply to the focused comment thread |
| `:` | reserved for a future command line |

Reserved prefixes:

| Prefix | Use |
|---|---|
| `g…` | goto (`gg`, `gt` / `gT` for tabs) |
| `]x` / `[x` | next / previous *x*: `f` file, `h` hunk, `c` comment thread, `s` section |
| `z…` | folds (expand/collapse threads, files) |

Adding a binding: make sure it doesn't collide with the tables above or another binding active in the same view, add it to the keymap with help text, and **list every new binding in the PR description** so the maintainer can approve it. Any binding an issue marks "TBD" is the maintainer's call; propose one in the PR and ask.

## Testing

- `go test ./...` must never touch the network or the real `gh` config. Use `internal/github/fake` or fixtures.
- **Table-driven tests** with the standard `testing` package; compare structs with `cmp.Diff`.
- **Fixtures** (recorded GraphQL JSON, sample diffs) live in a `testdata/` directory next to the test. Scrub tokens and personal data from anything recorded.
- **View rendering**: golden-file tests. Compare `View()` output, rendered at a fixed size, against `testdata/*.golden`, and regenerate with `go test ./... -update`. Strip ANSI codes unless the test is about styling.
- What must be tested: parsing and mapping logic, keymap/sequence handling, state transitions in `Update` (send a message, assert the new state and the returned command), and rendering of each view's main states (loading, empty, error, populated).
- No `time.Sleep` in tests. Inject clocks and timers.

## Code style

- `make fmt` (gofumpt + goimports) and `make lint` (golangci-lint) are authoritative; don't argue with them in `//nolint` comments unless you explain why on the same line.
- Error strings are lower-case with no trailing punctuation. Wrap with context: `fmt.Errorf("load pull request %d: %w", n, err)`.
- Doc comments on every exported identifier and every package. Otherwise comment *why*, not *what*; keep comments sparse.
- Keep functions small and names plain. Avoid `utils`/`helpers`/`common` packages.
- No global mutable state apart from build-time values such as `version.Version`.

## Workflow

1. **Branch** off an up-to-date `main`: `<type>/<issue#>-<short-slug>`, e.g. `feat/6-pr-list`, `fix/31-diff-scroll`. Types: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`.
2. **Commit** in small, coherent steps. Commit messages and PR titles use [Conventional Commits](https://www.conventionalcommits.org/): `feat(diff): add hunk navigation`, `fix(api): handle rate limit reset`. The scope is optional and is usually the area or package.
3. **Run `make check`.** It runs fmt-check, lint, race tests, tidy-check, govulncheck, actionlint, and build.
4. **Open a PR** against `main` using the PR template. It must include:
   - `Closes #N`
   - what changed and why
   - **how to try it** (the exact `strig` invocation and keys)
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
- `gh pr edit` fails with "Projects (classic) is being deprecated" on older `gh` versions. Update a PR body with `gh api -X PATCH repos/MikePors/strigidae/pulls/<N> -F body=@file.md` instead.
