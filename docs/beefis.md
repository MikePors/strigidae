# strig as a beefis unit

`strig --beefis`, run in a beefis pane, makes strig a beefis *unit*. strig refuses the flag outside a pane (`$BEEFIS_SESSION` unset).

strig doesn't speak beefis's socket protocol itself. It starts `beefis ctl unit <manifest>` as a child process, with the GitHub token variables removed from its environment. That process introduces strig to beefis with the manifest below and relays JSON lines:
- strig writes events to its stdin, one `{"name": ..., "data": {...}}` per line;
- commands beefis sends strig come back on its stdout, one per line.

Its stderr goes to strig's log file. When it exits (beefis closed the connection), strig shows an error once, and `:ask` says to restart strig.

## Event: `ask`

Emitted by `:ask <question>` in a pull request's diff.

| Field | Type | |
|---|---|---|
| `requestIdentifier` | text | `q1`, `q2`, … unique for this strig process |
| `repository` | text | `owner/name` |
| `number` | integer | the pull request |
| `head` | text | the commit the line numbers are from |
| `path` | text | the file |
| `side` | `LEFT` or `RIGHT` | the old or the new side of the diff |
| `line` | integer | the line (the last one of a selection) |
| `startLine` | integer, optional | the first line of a selection |
| `excerpt` | text, optional | the diff lines asked about plus three on each side, each prefixed `+`, `-` or a space |
| `question` | text | what the user typed |

The excerpt and question come from the pull request and the user. Whatever answers should treat them as untrusted data.

## Command: `answer`

| Field | Type | |
|---|---|---|
| `requestIdentifier` | text | the `ask` it answers |
| `text` | text | the answer, Markdown |
| `error` | text, optional | why there is no answer; shown instead of `text` |

An answer to an identifier strig didn't hand out is ignored. Answer text is untrusted and is sanitised like GitHub text before it is shown.

## Wiring

beefis's `iter` template shows the intended wiring. Both handlers use `scope = "unit-tab"`, so they run from the template of the tab that shows the unit, whichever tab is active.
- **`ask` → claude:** a handler turns the `ask` event into a prompt and types it into a Claude pane run under `beefis wrap`. The prompt asks Claude to start its reply with `[[strig:<requestIdentifier>]]`.
- **Claude's reply → strig:** Claude's Stop hook hands each reply to the wrap as an `output` event. A second handler looks for the marker and sends `answer` to the `strig` slot of the same tab. Replies without the marker are not answers.
