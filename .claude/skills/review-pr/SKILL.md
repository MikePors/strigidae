---
name: review-pr
description: Independent review of a strigidae pull request by a fresh agent, against the repo's rules. Usage `/review-pr <number>` reports findings in the terminal; `/review-pr <number> post` also posts them on the PR as one comment-only review with inline comments.
---

# Review a pull request

An independent second opinion on a PR, from a fresh agent that didn't write it. The arguments are the PR number and, optionally, the word `post`.

## 1. Gather context

- `gh api repos/MikePors/strigidae/pulls/<N>` gives the title, body, head SHA, base and branch. The body starts with `Closes #M`; read that issue with `gh api repos/MikePors/strigidae/issues/<M> -q .body`.
- `gh api repos/MikePors/strigidae/pulls/<N>/comments` and `.../reviews` show earlier review rounds. Resolved threads are settled, so don't repeat them. Do say if a fix is wrong.
- Check out the branch somewhere it won't disturb the user's work. The usual place is a temporary worktree: `git fetch origin pull/<N>/head && git worktree add --detach <tmp> FETCH_HEAD`. Remove it afterwards.
- Note what the issue and the PR body record as **maintainer decisions** (keys, wording, scope, accepted trade-offs). Reviewers must not re-litigate them.

## 2. Brief a fresh reviewer

Use the `Agent` tool with `subagent_type: "general-purpose"`, `model: "opus"` and `effort: "medium"` (higher only if the user asks). Don't use a fork; the point is a reviewer with no stake in the code. The prompt must stand alone:

- **What the PR is for**, in plain terms, and where the checkout is. Tell it to treat the checkout as read-only and run experiments in a `mktemp` copy.
- **Read first:** `CLAUDE.md`, the area guides it links (`internal/userinterface/CLAUDE.md`, `internal/github/CLAUDE.md`), the issue, the PR body, and `git diff origin/main...HEAD`.
- **The decisions to leave alone** (from step 1), labelled "already decided, don't re-report".
- **What to check:**
  - **Correctness.** Read the main changed files in full and trace at least one real path end to end. Look for:
    - stale or misrouted results (identity and sequence numbers on messages);
    - I/O outside a `tea.Cmd`;
    - state left behind (captured keys, flags that never clear);
    - off-by-one errors in scrolling and widths;
    - error handling that swallows failures.
  - **The repo's rules:**
    - untrusted text through `terminaltext`;
    - colours only from theme roles and styles;
    - the confirmation rules for anything that writes to GitHub;
    - the dependency directions;
    - the Naming rule: full English names, except `i`/`j`/`k`/`err`/`ctx`/`ok` and the listed exceptions.
  - **Tests.** Main view states, goldens that strip ANSI, and tests that can't fail. Prove a suspect test with a mutation in the temp copy.
  - **Scope.** Changes outside the issue belong under Follow-ups.
- **Each correctness finding needs a concrete failure scenario** (input or state, then the wrong output or crash), the `file:line`, and whether it should block merging. Rank the findings, most severe first, with simplifications last.
- **Never write to GitHub** unless step 3 says to post.

## 3. Report, and post only if asked

- **Without `post`:** summarise the findings for the user, most severe first, and stop. Change nothing.
- **With `post`:** after the report, post exactly ONE review: `gh api -X POST repos/MikePors/strigidae/pulls/<N>/reviews --input <mktemp file>`.
  - Use `event: "COMMENT"`, never `APPROVE` or `REQUEST_CHANGES`.
  - Set `commit_id` to the head SHA and write a short summary.
  - Add inline `comments` (`path`, `line`, `side: "RIGHT"`) only on lines that are in the diff; validate them first. Anything else goes in the summary.
  - If the post fails, say so and give the path of the saved payload. Don't retry another way.
- **Either way:** don't fix anything as part of this skill. Fixing is the author's next step.
