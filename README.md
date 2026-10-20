# strigidae

A terminal UI for reviewing GitHub pull requests, with vim keybindings.

The binary is called `strig`.

> **Status:** pre-alpha. Nothing works yet. See the [project board](https://github.com/users/MikePors/projects/1) and [milestones](https://github.com/MikePors/strigidae/milestones).

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

## License

[MIT](LICENSE)
