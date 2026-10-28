# Human tasks

Checks only the maintainer can do, such as live verification against real GitHub. Each item is removed once it's done.

## Live checks

- [ ] **`:discard-review` deletes a pending review on GitHub** (#139, merged in #143). The delete call (GraphQL `deletePullRequestReview`) has only been tested against the fake client.
  1. On a test PR, add two or three inline draft comments, so you have a pending review.
  2. Run `:discard-review`. The question should name the number of drafts. Answer `y`.
  3. strig should say `Pending review discarded`, the drafts should disappear from strig, and the pending review should be gone on github.com.
  4. Run `:discard-review` again. It should say `no pending review`.
  5. **Submitted elsewhere:** start a pending review again, run `:discard-review`, and leave the `y/N` open. Submit the review on github.com, then answer `y` in strig. strig re-reads before deleting, so it should say `no pending review` and delete nothing.
  6. **GitHub's answer for a submitted review** (outside strig). The code assumes deleting a review that is no longer pending gives an error other than not-found. Take the node ID of the review you just submitted (`gh api graphql -f query='{repository(owner:"<owner>",name:"<repo>"){pullRequest(number:<N>){reviews(last:1){nodes{id state}}}}}'`), then run `gh api graphql -f query='mutation{deletePullRequestReview(input:{pullRequestReviewId:"<id>"}){clientMutationId}}'`. Report the error GitHub returns.
