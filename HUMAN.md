# Human tasks

Checks only the maintainer can do, such as live verification against real GitHub. Tick each item off or delete it when it's done.

## Live checks

- [ ] **`:discard-review` deletes a pending review on GitHub** (#139, merged in #143). The delete call (GraphQL `deletePullRequestReview`) has only been tested against the fake client.
  1. On a test PR, add two or three inline draft comments, so you have a pending review.
  2. Run `:discard-review`. The question should name the number of drafts. Answer `y`.
  3. The drafts should disappear from strig, and the pending review should be gone on github.com.
  4. Run `:discard-review` again. It should say `no pending review`.
  5. **The submitted-elsewhere race:** start a pending review again, run `:discard-review`, and leave the `y/N` open. Submit the review on github.com, then answer `y` in strig. Note the message strig shows. The code expects GitHub to answer with something other than not-found here, but nobody has checked. If GitHub answers not-found, strig shows `no pending review`; otherwise it shows the error. Report which one happened.
