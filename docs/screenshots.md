# Screenshots for pull requests

PRs that change what's on screen include PNG screenshots. Render them from `internal/github/fake` data with the real theme, so they don't depend on anyone's GitHub account or terminal.

## Rendering

1. **A throwaway program.** Create an *untracked* program inside the module, for example `cmd/zz-shot/main.go`, so it can import `internal/…`. Delete it before running `make check` or committing.
   - It builds your view or component with fake data.
   - It sends the messages a real run would: `tea.WindowSizeMsg`, and the load results (run the returned `tea.Cmd`s synchronously).
   - It writes the styled output of `Render(width, height)` to `<name>.ansi`.

   Use `theme.Default()`, the same fixed Markdown style the tests use (see the tests in `internal/userinterface/components/markdown`), and a fixed clock.
   - Don't strip ANSI codes: Lip Gloss v2 emits full colour whatever the output.
   - Use fictional people and data, such as `octo-dev`, `rivka` and `sam-k`, with realistic PR titles. Never use real people.
2. **Convert** each file with [freeze](https://github.com/charmbracelet/freeze) into a folder outside the checkout (`shots=$(mktemp -d)`):

   ```sh
   go run github.com/charmbracelet/freeze@v0.2.2 <name>.ansi --language ansi --window --padding 20 --font.size 14 -o "$shots/<name>.png"
   ```

   If lines wrap in the image, raise freeze's `--width` until the frame is at least as wide as the render. Look at every PNG before using it.
3. **Clean up.** Delete the throwaway program and the `.ansi` files; `git status` must be clean. Never commit screenshots to your branch.

## Publishing

Screenshots live on the orphan `screenshots` branch, one folder per PR, and the PR body links them by commit so the links never change:

```sh
git fetch origin screenshots
git worktree add --detach ../strig-shots origin/screenshots
mkdir -p ../strig-shots/pr-<N> && cp "$shots"/*.png ../strig-shots/pr-<N>/
git -C ../strig-shots add pr-<N>
git -C ../strig-shots commit -m "screenshots for #<N>"
git -C ../strig-shots push origin HEAD:screenshots   # if rejected: git -C ../strig-shots pull --rebase origin screenshots, then push again
sha=$(git -C ../strig-shots rev-parse HEAD)
git worktree remove ../strig-shots
```

Link each image in the PR body's screenshot section with a caption saying what state it shows and at what size:

```markdown
![dashboard, 100x14](https://raw.githubusercontent.com/MikePors/strigidae/<sha>/pr-<N>/populated.png)
```

When a PR's UI changes again, publish to `pr-<N>-v2` and update the links.
