# GitHub boundary guide (`internal/github/...`)

Rules for the domain types, the `Client` interface, the real client in `api/`, and the fake in `fake/`. The root `CLAUDE.md` still applies.

- The UI depends only on the `github.Client` **interface** and the domain types in this package. GraphQL response structs stay private to `internal/github/api` and are mapped to domain types there.
- `internal/github/api` is the only package that talks to the network, and only `cmd/strig` imports it.
- Every `Client` method takes a `context.Context` first.
- Prefer GraphQL; review threads, resolving, and pending reviews only exist properly there. Use REST only where GraphQL lacks something (e.g. raw diffs).
- Never hardcode `github.com` as the host (see the root `CLAUDE.md`).
- API failures come back as typed errors (e.g. `github.ErrNotFound`, `*github.RateLimitError`) wrapped with `%w`. The UI decides how to display them.
- **Mutations** (comment, reply, resolve, react, merge) are implemented in `api/` like any other method, but tested only against `fake/`. Agents never call them against real GitHub; the maintainer verifies live.
- **Fixtures**: recorded GraphQL JSON lives in `testdata/` next to the test (scrubbed, per the root `CLAUDE.md`).
- When adding a `Client` method, add it to `fake/` in the same PR, with a way for tests to inject errors and inspect calls.
