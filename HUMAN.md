# Human tasks

Checks only the maintainer can do, such as live verification against real GitHub. Each item is removed once it's done.

## Live checks

- [ ] **`:edit-pull-request` edits a pull request's title and body on GitHub** (#174). The `updatePullRequest` mutation and the `viewerCanUpdate` field have only been tested against the fake client and fixtures.
  1. Open a test PR you may edit in strig and run `:edit-pull-request`. The editor should open with the title, a blank line, then the body.
  2. Change both, close the editor, and answer `y`. strig should say `Pull request updated` and the Overview should show the new title and description; check both on github.com.
  3. Run it again and close the editor without changes. It should say `no changes`.
  4. Delete the title line's text and close. It should show an error and send nothing.
  5. Open a pull request you can't edit (someone else's, in a repo where you only have read access) and run the command. It should say `you can't edit this pull request` before opening an editor.

- [ ] **`:copy-filename` reaches the system clipboard** (#172). Only the fake copier is tested.
  1. On macOS, inside tmux with no `set-clipboard` setting in your tmux config, open a pull request, select a file and run `:copy-filename`, then paste elsewhere. Report whether it pasted.
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
