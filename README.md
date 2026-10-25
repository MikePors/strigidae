# strigidae

A terminal UI for reviewing GitHub pull requests, with vim keybindings.

The binary is called `strig`.

> **Status:** pre-alpha. See the [milestones](https://github.com/MikePors/strigidae/milestones) for what is planned, and [CONTRIBUTING.md](CONTRIBUTING.md) to help.

## Building from source

Requires Go (any recent version; the `go` command fetches the exact toolchain pinned in `go.mod`).

```sh
make build        # builds ./bin/strig
./bin/strig --version
make test         # go test -race ./...
make check        # everything CI runs: fmt, lint, test, tidy, govulncheck, actionlint
make lint         # golangci-lint (tools are pinned in tools/<name>/go.mod)
```

Or install directly:

```sh
go install github.com/MikePors/strigidae/cmd/strig@latest
```

## Dashboard

Run `strig` outside a git checkout, or in one with no GitHub remote (and without `-R`), to see the dashboard: one tab per entry in the config (by default a single "Me" tab), each tab showing its sections stacked, with pull requests from any repository. `g t` / `g T` switch tabs, `]` / `[` jump between sections, `enter` opens the pull request, `o` opens it in the browser and `R` refreshes the tab, and `/` searches the rows: type to jump to the first match, `enter` keeps the search (matching rows get a marker), `n` / `N` move to the next / previous match and `esc` clears it. The repository's pull request list and the Files pane of a pull request search the same way. Inside a checkout with a GitHub remote, or with `-R owner/repo`, strig shows that repository's pull requests instead. A checkout whose remote is on a host strig doesn't know (another forge, or a GitHub Enterprise host you are not signed in to) still exits with an error; pass `-R owner/repo` there.

## Checks, merge state and reviewers

In a pull request, the Overview lists the head commit's checks (failing first) in a "Checks (N)" section, reached with `]` / `[`. `:checks` opens the same list as a picker; `enter` opens the selected check in the browser. The header shows conflicts, "behind base" or "blocked" when one stands in the way of merging, and each reviewer with a mark: `✓` approved, `✗` changes requested, `•` commented, `?` requested and not yet reviewed.

## Clipboard

`:copy-filename` copies the selected file's path using the OSC 52 terminal escape, which also works over SSH. Terminals that ignore OSC 52 copy nothing. Inside tmux, add `set -g set-clipboard on` to `~/.tmux.conf`.

## Configuration

strig reads an optional YAML config (tabs and the search queries behind their sections) from `$XDG_CONFIG_HOME/strig/config.yml` (default `~/.config/strig/config.yml`); `--config <path>` overrides it and `strig --config-path` prints the location in use. See [docs/config.md](docs/config.md) and [docs/config.example.yml](docs/config.example.yml). The yes/no options (`wrapscan`, `hlsearch`, and `wrap` for long diff lines) can be set there or with `:set` while strig runs. Colour themes (bundled ones, or any Base16/Base24 scheme file; `strig --list-themes`; `:theme <name or file>` switches while strig runs) are described in [docs/themes.md](docs/themes.md).

## License

[MIT](LICENSE)
