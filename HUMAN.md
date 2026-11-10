# Human tasks

Checks only the maintainer can do, such as live verification against real GitHub. Each item is removed once it's done.

## Live checks

- [ ] **A hunk that starts inside a comment is coloured from the whole file, in a real terminal** (#262). The tests use an in-memory blob reader and check token types, not the colours on screen or a real checkout.
  1. In a checkout of a repository (no `-R`), run `strig` and open a pull request that modifies a file inside a long block comment or docstring, so the hunk starts below the line that opens it. Open the file: the hunk's comment lines may flash code colours, then turn the comment colour without a pause or a message. Take a before/after screenshot (the "before" is the same file opened with `strig -R owner/repo`, which keeps per-hunk colours).
  2. Use a pull request from a fork, or whose head you have never fetched: the colours should still change after a moment (strig fetches `refs/pull/<number>/head` once). Afterwards `git status`, `git branch -a` and `.git/FETCH_HEAD` look as before.
  3. Open a modified file over 1 MiB, and one in a pull request while offline: both keep per-hunk colours, and `strig --log-path`'s file says why.
  4. Report the language, whether the colours changed, how long it took, and any pause or message.

- [ ] **An extensionless script is coloured by its shebang, in a real terminal** (#261). The tests check token types, not the colours on screen.
  1. Run `strig -R owner/repo` and open a pull request that changes (or adds) an extensionless script with a `#!/usr/bin/env python3` or `#!/bin/sh` first line, such as `bin/release`. Open the file: the diff should be coloured as Python (or shell), the shebang line included.
  2. Open a file with no extension and no shebang (a `NOTES` file, or a Go file called `main`): it stays one plain colour. In a script whose diff has several hunks, a hunk that starts further down than line 1 is coloured like the first one, whichever you scroll to first, and also when the first hunk is marked reviewed and hidden.
  3. Open a change that swaps the shebang (`#!/bin/sh` for `#!/usr/bin/env python3`): the deleted lines are coloured as shell and the added lines as Python.
  4. Report the file, the language colours you saw, and any extensionless file coloured as the wrong language.

- [ ] **A multi-line comment or string is coloured whole in a hunk, in a real terminal** (#259). The tests check token types, not the colours on screen.
  1. Run `strig -R owner/repo` and open a pull request whose diff has a block comment over several lines (`/* … */`, a Python docstring or a raw string) that opens and closes inside one hunk, as added lines. Every line of it should have the comment (or string) colour, not the code colours.
  2. Open a hunk where a comment opens on a deleted line and closes on a context line: the deleted line is a comment, the context line is not. Scroll a file of several thousand lines: drawing its first screen should not pause noticeably.
  3. Report the language, the colours you saw and any pause.

- [ ] **Fetching a pull request's head never prompts, over HTTPS and SSH** (#267). Only local repositories stand in for the remote in the tests. Reachable from strig once #262 lands; until then, check what strig will run: `git fetch --no-write-fetch-head --refmap= --no-tags --no-auto-maintenance --no-recurse-submodules --quiet -- <remote> refs/pull/<number>/head` with stdin closed, `GIT_TERMINAL_PROMPT=0` and `GIT_SSH_COMMAND='ssh -o BatchMode=yes'`.
  1. Once #262 is merged: in a checkout of a repository you can reach over HTTPS with no stored credentials (`git credential reject`, or a private repository), open a pull request whose blobs are not local and open a modified file. strig must not stop, ask for a username, or draw anything over the TUI; the file just keeps its per-hunk colours.
  2. Repeat with an SSH remote, a passphrase-protected key and no agent (`env -u SSH_AUTH_SOCK strig`). It must not ask for the passphrase or a host key, and must not hang.
  3. With a working login (HTTPS credential helper, or SSH key in an agent), open the same file: the whole-file colours should appear. Afterwards `git status`, `git branch -a`, `git for-each-ref` and `.git/FETCH_HEAD` must look exactly as before.
  4. Report the transport, whether anything prompted or hung, and any ref or `FETCH_HEAD` change.

- [ ] **`o` opens the browser without the token variables, in a real terminal** (#145). Only a fake command runner is tested.
  1. With no `GH_BROWSER`, no `BROWSER` and no gh config `browser` set, open a pull request in strig and press `o`. The default browser should open the pull request and nothing should be printed over the TUI. Do this on Linux, on macOS and on Windows (Windows now runs `rundll32 url.dll,FileProtocolHandler` as a child process instead of an in-process call).
  2. Run `GH_BROWSER='sh -c "env > /tmp/env.txt" --' GH_TOKEN=x strig`, press `o` on a pull request, then check `/tmp/env.txt`: it must contain no `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN` or `GITHUB_ENTERPRISE_TOKEN`, and should still have `PATH` and `HOME`.
  3. Report the platform, whether the browser opened, anything drawn over the TUI, and any token variable found in `/tmp/env.txt`.

- [ ] **Comments pane in a real terminal** (#233).
  1. Run `strig -R owner/repo`, open a test PR containing PR comments, review summaries, resolved and outdated threads, and drafts. Select Comments below Overview, then try `:comments unresolved` and `:comments all`.
  2. Search for text in a reply with `/`, move with `n` / `N`, and confirm the matching conversation expands. Read a long conversation with `ctrl+d` / `ctrl+u`; resize below 80 columns and use `h` / `l` to switch focus.
  3. Press `enter` on a current thread, including a deleted-side thread and a hidden resolved thread. Check the exact code/thread is selected. Run `:comments` to return. An outdated thread must stay in Comments with a notice; `o` must open its permalink. Check a review summary's permalink too.
  4. Reply to and resolve a test review thread from Comments, checking the confirmation and the result on GitHub. Repeat with local plugin results: writes and browser opening must be refused. Report any wrong target, missing content, or terminal overflow.

- [ ] **Local threads survive restarting strig** (#202). Only files in a temporary directory are tested.
  1. Configure a `lines` plugin with `show: thread` (for example `docs/plugins.md`'s `ask`), open a pull request, move onto a diff line and run `:ask why?`. Wait for the answer, then quit (`:qa`) and start `strig` again on the same pull request. The question and the answer should be back on the same lines.
  2. Run `:ask` with a plugin that takes a while, quit while the thread says `waiting for an answer…`, start `strig` again and open the pull request. The thread should say `no answer: strig was closed`.
  3. Check `~/.local/state/strig/local-threads/<host>/<owner>/<name>/<number>.json` exists, is readable only by you, and that deleting the directory removes the saved answers. Report anything on screen that looks different after the restart.

- [ ] **`Tab` completes on the command line in a real terminal** (#208). Only synthetic key events are tested; terminals differ in what they send for `shift+tab`.
  1. In a pull request list, press `:` then `Tab`. The first command fills in and a one-line list of every command appears just above the command line, the current one highlighted. `Tab` and `shift+tab` should move through it and wrap.
  2. Type `:set no` then `Tab`, and `:theme ` then `Tab`. Option and theme names should cycle. Type a character: the list closes and the match stays.
  3. Open a pull request: `:` `Tab` should now offer its commands (`:approve`, `:merge`, …) and not `:create`. Shrink the terminal to under 30 columns and check the list scrolls with `…` and never wraps.
  4. Report any terminal where `shift+tab` does nothing or types a character.

- [ ] **`:create` and `:create-draft` create pull requests on GitHub** (#173). The `createPullRequest` mutation and the branch listing have only been tested against the fake client and fixtures.
  1. In a test repository, push a branch with at least one commit that differs from `main`. Run `strig` in the checkout and open the pull request list.
  2. Run `:create`. The head picker should open with your current branch under the cursor; `enter`. The base picker should not list that branch and should have the default branch under the cursor; `enter`. The editor opens empty: write a title, a blank line and a body, close it, answer `y`. The new pull request should open in strig and appear in the list; check title, body, head, base and that it is not a draft on github.com.
  3. Push a second branch and run `:create-draft`. No `y/N` should be asked; check on github.com that it is a draft.
  4. Run `strig -R <owner>/<repo>` from outside the checkout (or with `-R`) and run `:create`: no head should be preselected.
  5. `esc` in each picker should cancel quietly; an empty title in the editor should show an error and create nothing.
  6. **Error case:** run `:create` again with the same head as step 2 (a pull request is already open) and, separately, a head branch that has no commits beyond the base (create one with `git branch no-commits main` and push it). The status bar should show GitHub's message. Report the wording.

- [ ] **`:edit-pull-request` edits a pull request's title and body on GitHub** (#174). The `updatePullRequest` mutation and the `viewerCanUpdate` field have only been tested against the fake client and fixtures.
  1. Open a test PR you may edit in strig and run `:edit-pull-request`. The editor should open with the title, a blank line, then the body.
  2. Change both, close the editor, and answer `y`. strig should say `Pull request updated` and the Overview should show the new title and description; check both on github.com.
  3. Run it again and close the editor without changes. It should say `no changes`.
  4. Delete the title line's text and close. It should show an error and send nothing.
  5. Open a pull request you can't edit (someone else's, in a repo where you only have read access) and run the command. It should say `you can't edit this pull request` before opening an editor.

- [ ] **`:copy-filename` and `:copy-url` reach the system clipboard** (#172, #171). Only the fake copier is tested.
  1. On macOS, inside tmux with no `set-clipboard` setting in your tmux config, open a pull request, select a file and run `:copy-filename` and `:copy-url`, then paste elsewhere. Report whether it pasted.
  2. On Linux (Wayland with `wl-copy`, and X11 with `xclip` or `xsel` if you can), do the same inside and outside tmux. Report which tool was used and whether it pasted.
  3. Check nothing was printed on the screen, and `strig --log-path` shows no clipboard error.

- [ ] **`:discard-review` deletes a pending review on GitHub** (#139, merged in #143). The delete call (GraphQL `deletePullRequestReview`) has only been tested against the fake client.
  1. On a test PR, add two or three inline draft comments, so you have a pending review.
  2. Run `:discard-review`. The question should name the number of drafts. Answer `y`.
  3. strig should say `Pending review discarded`, the drafts should disappear from strig, and the pending review should be gone on github.com.
  4. Run `:discard-review` again. It should say `no pending review`.
  5. **Submitted elsewhere:** start a pending review again, run `:discard-review`, and leave the `y/N` open. Submit the review on github.com, then answer `y` in strig. strig re-reads before deleting, so it should say `no pending review` and delete nothing.
  6. **GitHub's answer for a submitted review** (outside strig). The code assumes deleting a review that is no longer pending gives an error other than not-found. Take the node ID of the review you just submitted (`gh api graphql -f query='{repository(owner:"<owner>",name:"<repo>"){pullRequest(number:<N>){reviews(last:1){nodes{id state}}}}}'`), then run `gh api graphql -f query='mutation{deletePullRequestReview(input:{pullRequestReviewId:"<id>"}){clientMutationId}}'`. Report the error GitHub returns.

- [ ] **`:react` adds and removes a reaction on GitHub** (#156). The `addReaction` / `removeReaction` calls have only been tested against the fake client.
  1. On a test PR with a review comment in a thread, open it in strig and move the diff cursor onto the comment's text (not the thread's first line).
  2. Run `:react`. The picker should be titled with the comment's author and list the 8 reactions with their counts. Choose `+1` with `enter`.
  3. strig should say `reacted 👍`, and the comment should show 👍 with a count in strig and on github.com.
  4. Run `:react +1` on the same comment. strig should say `removed 👍`, and the reaction should be gone on github.com.
  5. Run `:react` with the cursor on a diff line. It should say `move onto a comment to react to it`.


- [ ] **`:react` on Overview targets the description and top-level comments** (#158). The `addReaction` / `removeReaction` calls have only been tested against the fake client.
  1. Open a test PR that has a description, top-level comments, a review summary and a review thread.
  2. In Overview run `:react`. The first picker should list Description, then top-level comments with author and a short body snippet; it must omit review summaries and review threads. Choose the description, then choose `+1` in the 8-reaction picker.
  3. strig should say `reacted 👍`, and the comment should show 👍 with a count in strig and on github.com.
  4. Run `:react +1` on the same comment. strig should say `removed 👍`, and the reaction should be gone on github.com.
  5. On Overview run `:react heart`, choose a top-level comment, and verify the reaction toggles without a second picker. Escape at either picker stage in separate attempts and verify GitHub is unchanged. Run `:react bogus` and verify it reports the valid names without opening a picker.
  6. Repeat with the diff unavailable and verify Overview reactions still work. While a write is busy, invoke `:react` and verify no picker opens. Remove a target between opening its picker and choosing it; verify it refuses without writing. If GitHub times out after a write may have landed, verify the appropriate Overview or review-thread content reloads.
  7. Deterministic UI capture fixture: `go test ./internal/userinterface/views/workspace -run TestOverviewReactionTargetPickerGolden`; the expected terminal screen is `internal/userinterface/views/workspace/testdata/overview_react_targets.golden` and can be regenerated with `UPDATE_GOLDEN=1`.

- [ ] **Plugin commands in a real terminal** (#200). The runs, the selection and the quit are tested with fake programs and key events, never in a real terminal.
  1. Put a small plugin in your config, for example a program `~/bin/strig-echo` (`chmod +x`) that starts with `#!/bin/sh` and holds `cat > /tmp/strig-request.json; echo '{"name":"notice","data":{"text":"got it"}}'`, and `plugins: [{command: echo, on_run: ["~/bin/strig-echo"], target: lines}]`. Open a pull request, select a file, move onto a diff line and run `:echo hello there`. The status bar should show a spinner briefly and then `got it`. `/tmp/strig-request.json` should have `"arguments":"hello there"`, your repository (with the right `host`), the pull request number and head, the file path, `side`, `line`, `startLine` and an `excerpt` of the line with three lines either side, each starting with `+`, `-` or a space.
  2. Press `V`, `j` twice, then `:` and run `:echo`. The command line should open over the selection; `esc` should close it and leave the selection (try that first). The request should hold `startLine` and `line` of the selection, and the selection should end after the run. Report anything odd about how the selection looks while the command line is open.
  3. Add a second plugin with `target: pull-request` that is a `#!/bin/sh` script ending in `exec sleep 30` (the `exec` matters: strig kills only the plugin process, so a plain `sleep 30` child would outlive it and keep running after you quit); it prints its notice before the `exec`. Run it from the Files pane and the Overview: the spinner should run for the whole time. Run it a second time while the first is still going: it waits its turn (the spinner stays on until both are done). Close the pull request with `:q` meanwhile: nothing should be shown when it finishes, and the plugin process should keep running until it ends (`pgrep -f` the program).
  4. Start the slow plugin and quit with `:qa` during the run: strig should exit at once (well under two seconds) and the plugin process should be gone (`pgrep`). Start two runs (one waiting) and quit: the second must never start.
  5. Make a plugin that runs `cat /dev/tty` (and one that prints `hi` to `/dev/tty`). Run it: it should fail at once (no such device or address), type nothing into strig, and nothing should be drawn over the screen. Report the status bar text.
  6. Make a plugin that prints `{"name":"notice","data":{"text":"\u001b[2J\u001b]0;pwned\u0007x"}}`: the status bar should show harmless text, the screen must not clear and the terminal title must not change.

- [ ] **Plugin answers as local threads in a real terminal** (#201). The thread, its states and its refusals are tested with fake runners and key events, never in a real terminal.
  1. Put a plugin in your config, for example `plugins: [{command: ask, on_run: ["~/bin/strig-ask"], target: lines, show: thread}]`, where `~/bin/strig-ask` is a `#!/bin/sh` script that runs `cat >/dev/null; sleep 3; echo '{"name":"answer","data":{"text":"It is **bounded** by `Retry`."}}'`. Open a pull request, move onto a diff line and run `:ask why does this retry?`. A thread should appear under the line at once: `you` with your question, then `answer (local, not on GitHub)` with `waiting for an answer…` in a muted colour. After three seconds the answer replaces the waiting line, drawn as Markdown. Report how it looks next to a real thread.
  2. Make the script print `{"name":"error","data":{"text":"no model"}}` instead: the second comment should read `no answer: no model` in the error colour. Run with `timeout: 1s` and the sleep: it should read `no answer:` followed by the timeout text.
  3. With the cursor on the thread (header, then each comment), try `r`, `x`, `e`, `D`, `:react +1`, `:resolve`: each says `this answer is local to strig, not on GitHub: nothing to <action>`, asks nothing and opens no editor. On a real thread in the same file, `x` still asks `y/N`.
  4. Run `:ask` while the sleep is running, then `:q` before it ends, and open the same pull request again from the list: the thread should be there and, once the run ends, answered. Quit strig: the answer is gone on the next start (it is kept in memory only).
  5. Push a new commit to the pull request's branch (or use another pull request head), press `R`: the thread should show `[outdated]`, muted, after the last row of its file. Report whether that reads clearly.
  6. Make the script print an answer with terminal escapes (`\u001b[2J\u001b]0;pwned\u0007x`): the screen must not clear and the terminal title must not change.

## #92 — cached list startup

- With `gh` logged in (no token environment override), run `strig -R OWNER/REPO`,
  wait for the list, and exit with `:q`. Start it again with a slow or disconnected
  network: cached rows should show `(stale)` immediately and remain on transient
  failure. Press `R` after reconnecting; fresh rows should replace them and the
  label should disappear.
- Repeat with `strig` outside a checkout for dashboard sections, and switch tabs
  with `g t`. Report any cursor jumps or labels clipped at your terminal size.
- Switch `gh` accounts on the same host and restart: the previous account's rows
  must not appear. Repeat with a token environment override: no cache is reused.
- Stop strig, delete its cache directory (see README), and restart: normal live
  loading should work. Report OS, terminal size, and any unexpected behavior.

- [ ] **`FileText` reads a file's text at a commit from real GitHub** (#260). The GraphQL `object(expression: "<commit>:<path>")` query has only been tested against hand-written fixtures. Nothing on screen uses it yet, so this can be done once #262 lands and highlights from the whole file; until then it needs a throwaway `main` that calls `network.New(...).FileText`.
  1. Against a real pull request's head commit (the full 40-character SHA), read a small text file the pull request touches. The returned text should match `git show <sha>:<path>` exactly, including line endings and a missing final newline.
  2. Read a binary file (for example a PNG) and a text file over about 1 MB. Each should fail with `ErrNoFileText`, not `ErrNotFound`. Report the size at which a text file starts to come back as truncated, if it does.
  3. Read a path that does not exist at that commit, a path that was added by a later commit, and a directory path. Each should fail with `ErrNotFound`.
  4. Report any case where the text came back wrong or the error was not the one above.

## #36 — image rendering prototype

- Run `go run ./cmd/imageprobe -protocol kitty` in Ghostty and
  `go run ./cmd/imageprobe -protocol iterm2` in WezTerm. Press `r` once the
  screen appears. Report terminal version and whether the gradient appears.
- Scroll with `j`/`k` until the image is partially and fully outside the pane.
  Toggle the covering overlay with `h`, resize, and press `r` again. Report
  missing images, stale pixels, overlap with text, or literal placeholder glyphs.
- Quit with `q`; check that the shell has no remaining image or altered cursor.
  This checks visible artifacts only: the queued Kitty delete may flush after
  the alternate screen has been left, so it does not prove image-store cleanup.
  The iTerm2 path is a diagnostic for repaint ordering, not a completed fallback.
  See `docs/design/36-inline-images.md` for the implementation gate.
- Attach screenshots or recordings and terminal versions. Record TERM,
  COLORTERM and any multiplexer. Run directly in each terminal first: truecolor
  is needed for the Kitty identifier, and the probe does not wrap tmux APC
  passthrough. Also run the Kitty probe in WezTerm to check its installed version.
