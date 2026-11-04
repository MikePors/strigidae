# PR Comments pane — design proposal

Give reviewers one place to scan every conversation on a pull request, read
replies, and jump to the relevant code. The pane is available through a new
Comments entry immediately below Overview in the Files sidebar, and through
`:comments` from anywhere in the PR workspace. Selecting it replaces the right
pane; the existing two-pane layout and focus keys stay familiar.

## Terminal sketch

Illustrative content; counts refer to conversations, not individual replies.

```text
open #7 Add retry policy to sync client
alice · changes requested · main ← feat/retry
╭─ Files 0/5 viewed ─────────╮╭─ Comments · 6 conversations ─────────────────────────╮
│  Overview                 ││ All · 2 unresolved · 1 draft                         │
│> Comments (6)             ││                                                      │
│    M internal/sync/…      ││   PR conversation · bob · 2d ago                     │
│    A internal/sync/…      ││     Could you add a test for the jitter bounds?       │
│    D internal/sync/…      ││                                                      │
│    R docs/backoff.md      ││   Review · carol · 1d ago · changes requested         │
│    M README.md            ││     The cap should be configurable.                  │
│                           ││                                                      │
│                           ││ > internal/sync/client.go:14 · 2 comments · 4h ago    │
│                           ││   [unresolved]                                       │
│                           ││   bob · 5h ago                                       │
│                           ││     Why retry here?                                  │
│                           ││   alice · 4h ago                                     │
│                           ││     Handles flaky links.                             │
│                           ││                                                      │
│                           ││   README.md:1 · erin · 1h ago · [resolved]            │
│                           ││     Typo.                                            │
│                           ││                                                      │
│                           ││   internal/sync/client.go:3 · dave · 2d ago           │
│                           ││     This moved. [outdated]                           │
│                           ││                                                      │
│                           ││   internal/sync/retry.go:28 · me · [draft]            │
│                           ││     Consider capping the delay.                      │
╰───────────────────────────╯╰──────────────────────────────────────────────────────╯
j/k move  enter code  r reply  x resolve  / search  :comments unresolved
```

The sketch shows the Comments selection retained in the sidebar while keyboard
focus is on the right. Only the focused pane has the bright border. Use the
existing theme's selected, author, muted, pending, and error styles. Keep text
labels for every state so meaning does not depend on colour.

## What appears

One selectable item represents a top-level PR comment, a review summary, or a
review thread (including its replies). Include all files, file-level threads,
outdated threads, resolved threads, and the viewer's pending review comments.
Include review verdicts even when their body is empty; omit empty submitted
comment-only review wrappers, matching Overview's current rule. The PR
description stays in Overview.

Each collapsed item shows location or kind, author, relative activity time, a
one-line body preview, and applicable state labels. Thread headers show the
total comment count. The selected item expands its full Markdown body and all
replies in chronological order, using the existing comment/thread renderers.
Selecting another item collapses the previous one. Long bodies remain readable
through viewport scrolling; keep the selected item's header visible when first
selected, without requiring its entire body to fit.

Default order is published conversations by latest activity, newest first;
within a conversation, oldest comment first. Own draft-only conversations form
a labelled Drafts group above the published list. Draft replies remain in their
parent thread with an explicit `draft, not submitted` label. Use stable IDs to
break ties. Local plugin threads appear in a separate Local group below the
GitHub conversations, with waiting/answered/failed labels; they do not contribute
to GitHub conversation or unresolved counts.

`Comments (N)` counts GitHub conversations, including draft-only conversations,
once each. The header separately reports unresolved GitHub threads and draft
comments. Show no final count while a required source is still loading. Under a
filter, show `3 of 6 conversations`; unresolved counts exclude top-level comments,
review summaries, and local threads. Outdated and unresolved are independent.

## Navigation and actions

| Input | Behaviour in Comments |
| --- | --- |
| `j` / `k`, counts | Select next/previous conversation and expand it. |
| `gg` / `G` | Select first/last conversation, respecting counts as existing lists do. |
| `ctrl-d` / `ctrl-u`, page keys | Scroll the content, including long selected conversations. |
| `h` / `l` | Focus Files / right pane, as today. |
| `enter` | For a current inline thread, open the file and position the diff at its exact thread and side. For a file-level thread, open the file at that thread. For other items, keep the expanded conversation in view. |
| `o` | Open the selected conversation's GitHub permalink. |
| `r` | Reply to the selected GitHub review thread through the existing editor flow. |
| `x`, `:resolve`, `:unresolve` | Apply to the selected GitHub review thread, using the existing fresh-target and write guards. |
| `C` | Compose a top-level PR comment using the existing flow. |
| `/`, `n` / `N`, `esc` | Existing incremental search semantics, scoped to Comments. |
| `R` | Refresh PR detail and review threads together. |

`:comments` opens and focuses this pane and retains its previous filter,
selection, and scroll. `:comments all` and `:comments unresolved` explicitly set
the filter; offer both arguments in completion. The unresolved filter includes
only unresolved GitHub review threads, including outdated ones. It is independent
of the diff's `hideresolved` and `hideviewed` options. Keep the full comment index
available even when files or hunks are hidden in the diff.

Search matches author, full path, state labels, and all comment bodies, including
replies hidden in collapsed items. A match selects and expands its conversation
and scrolls to the matching comment. Keep separate search state from Files and
the diff and honour `wrapscan` and `hlsearch`.

For outdated threads without an exact current anchor, `enter` keeps Comments
open and shows `No current diff location; press o to open on GitHub.` Display
the saved original hunk with an explicit `original context` label when available.
Never silently jump to the same line number in newer code. If a file is hidden
by viewed-file filtering, explicitly reveal it for the jump. Returning through
`:comments` restores the previous conversation and scroll position.

Top-level comments and review summaries do not offer thread replies or resolution.
Use `C` for a PR response. Initially, editing, deleting, and reacting continue
through their existing diff targets; the Comments help must not advertise them.
Do not infer a target comment from a selected multi-comment conversation.
Local items have no GitHub write actions or browser permalink.

## Small terminals and loading states

Below the existing 80-column breakpoint, show only the focused pane. `enter` on
the sidebar's Comments row focuses the full-width Comments pane; `h` returns to
Files. Wrap bodies to the available width, shorten long paths from the left, and
preserve state labels on their own line when needed. Do not add a third column.

Load PR comments/review summaries and threads independently. Show available
items immediately with `Loading review threads…` until the other source arrives.
On a source failure, retain successful content and show `Review threads could
not be loaded. R retry.` Do not turn a failed load into `No comments yet.`
If a refresh fails, keep the prior items with a visible stale/error notice.

Empty copy: `No comments yet. Press C to start the conversation.` Filter-empty
copy: `No unresolved threads. :comments all shows every conversation.` A search
miss uses the existing search notice. After refresh or resolution, preserve the
selection by stable ID; if it disappears from the active filter, select the next
visible neighbour, or the previous one at the end. Do not reorder the list while
the user is reading; update ordering when the refresh completes, preserving the
selected ID and its viewport anchor.

## Implementation fit and acceptance

Add an explicit right-pane content mode for Overview, Comments, and File rather
than reserving another file-index sentinel. Existing workspace logic assumes
`selection() > 0` means a diff; audit those gates before introducing the row.
Keep Comments cursor/search/scroll separate from file selection and diff state.
Build a derived index over `PullRequestDetail.Comments`, `.Reviews`, and the
loaded `ReviewThread` collection; retain typed stable references to source items.
Reuse `conversation`'s review inclusion rules and existing Markdown, thread,
theme, terminal-sanitization, and editor components.

Extend target resolution to accept a selected Comments review thread. Reuse the
existing mutation commands, fresh checks, local-thread refusal, and busy guards;
do not introduce a second write path. Browser actions must target an actual
comment URL; an empty review verdict currently needs a review permalink added
to the data model before that action can work reliably.

Acceptance checks for implementation:

- Every PR comment, eligible review summary, and review thread is reachable
  exactly once; replies remain grouped, drafts and local results remain explicit.
- Search finds text in a collapsed reply, including in resolved/outdated threads.
- Jumping selects the correct file, thread, and old/new side; absent anchors do
  not select unrelated lines. Hidden files can be reached.
- Resolve/reply act on the selected stable thread even after list movement or
  asynchronous refresh, and retain the existing permission/error behaviour.
- Refresh and filtering preserve selection where possible; partial failures
  remain visibly partial and never erase previously loaded content.
- Golden views cover a mixed list, a long selected thread, narrow terminals,
  draft/local labels, empty and filtered-empty states, and source failures.

Implementation lives in the workspace Comments pane. Current review activity uses
comment creation/submission times because the API model does not track edit times.
