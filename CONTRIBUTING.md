# Contributing

Thanks for helping with strigidae. This page gets you from a fresh clone to an open pull request. The rules themselves live in [CLAUDE.md](CLAUDE.md) and the area guides it links. They're written for humans and AI agents alike, so read them before your first issue.

## Setup

1. **Go.** Any recent version; the `go` command fetches the exact toolchain pinned in `go.mod`.
2. **GitHub CLI.** Install `gh` and run `gh auth login`. strig uses your `gh` login, and so do the scripts and skills here.
3. **Optional:** `shellcheck`, for full parity with CI's actionlint step.
4. Clone and check that everything passes:

   ```sh
   gh repo clone MikePors/strigidae
   cd strigidae
   make check    # exactly what CI runs: fmt-check, lint, race tests, tidy, govulncheck, actionlint, build
   make build && ./bin/strig --version
   ```

You'll have write access as a collaborator, so push branches to this repository rather than to a fork.

## Picking an issue

- Open issues are grouped by milestone. Take one that nobody is assigned to, assign yourself, and say so on the issue if it's been discussed.
- Issues describe the behaviour wanted and the maintainer's decisions. Those decisions are settled, so don't re-litigate them in the PR.
- If an issue is ambiguous about something you'd see on screen (keys, layout, wording, config), ask on the issue before building it.

## Working on an issue with Claude Code

Point Claude at the issue, for example:

> Implement #12. Follow CLAUDE.md and the area guides. Branch off origin/main, keep to the issue's scope, run `make check`, and open a PR.

Claude loads `CLAUDE.md` automatically, and the area guides when it reads files in those directories. Things worth checking in what it produces:

- **It stays read-only against GitHub.** Agents never post comments, reviews or reactions, and never merge or edit other people's PRs. Features that write to GitHub are tested against `internal/github/fake`, and the maintainer verifies them live.
- **Names are spelled out in full English** (the Naming section of CLAUDE.md). This is the most common thing review catches.
- **It builds the simplest version** and lists extras under Follow-ups.

## Pull requests

- Branch `<type>/<issue>-<slug>`, Conventional Commits, and the PR template; details are in CLAUDE.md's Workflow section.
- Run `make check` before every push.
- Visible UI changes need a screenshot: see [docs/screenshots.md](docs/screenshots.md).
- **Review:** run `/review-pr <number>` in Claude Code for an independent review. It reports in your terminal. `/review-pr <number> post` puts the same findings on the PR as one comment-only review. Fix findings in new commits, reply in each thread with the fixing commit, and resolve it.
- The maintainer merges, with merge commits. Never merge, push to `main`, or force-push a branch you didn't create.

## Odd tooling

- Older `gh` versions fail on `gh pr edit` ("Projects (classic) is being deprecated"). Edit a PR body with `gh api -X PATCH repos/MikePors/strigidae/pulls/<N> -F body=@body.md`.
- `gh issue view` fails the same way. Read an issue with `gh api repos/MikePors/strigidae/issues/<N> -q .body`.
