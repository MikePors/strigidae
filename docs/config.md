# Configuration

strig reads a YAML file at `$XDG_CONFIG_HOME/strig/config.yml` (falling back to
`~/.config/strig/config.yml`). `strig --config <path>` uses another file, and
`strig --config-path` prints the location that would be used.

A missing file is fine: strig uses the defaults below. A file named with
`--config` must exist. A file with a mistake stops strig before the screen
starts, with the file, line and problem, for example:

    strig: /home/me/.config/strig/config.yml:7: unknown key "limt" in a section (valid keys: name, query, limit, sort, order)

Unknown keys are errors so typos don't pass silently.

Other rules:

- The file must be a single YAML document and at most 1 MiB.
- YAML anchors, aliases (`*name`) and merge keys (`<<`) are not supported and
  are errors.
- `name`, `user` and `query` accept any plain text value, so `user: 12345`
  works; they are trimmed of surrounding spaces. `limit` must be an unquoted
  integer.
- Tab names must be unique, and section names must be unique within a tab.
- For YAML syntax errors the line comes from the YAML parser and can point at
  the start of the enclosing block rather than the exact line of the mistake. A commented example is in
[`config.example.yml`](config.example.yml).

## Shape

```yaml
tabs:
  - name: Me
  - name: Alice
    user: alice
    sections:
      - name: Needs review
        query: is:pr is:open review-requested:{user}
        limit: 50
        sort: updated
        order: desc
```

### `theme`

Optional. A bundled theme's name (`strig --list-themes`) or the path of a
Base16/Base24 scheme file; unset means the built-in theme. A relative path is
taken from the config file's directory, and `~` is expanded. See
[themes.md](themes.md).

### Options

Optional yes/no keys, written at the top level next to `theme`. Each is also
a `:set` option (`:set nowrapscan`, `:set wrap`, `:set` alone to show
them), which changes it for the running session without touching the file.
The value must be an unquoted YAML boolean: `true` or `false` (`True` and
`TRUE` also work). Quoted values and `yes` / `no` are errors naming the line, for example
`config.yml:3: wrapscan must be true or false`.

| Key | Default | Meaning |
|---|---|---|
| `wrapscan` | `true` | `n` / `N` continue past the last or first match of a search; with `false` they stop and say `no more matches below` / `above` |
| `hlsearch` | `true` | rows matching a search are marked; with `false` they are not (the search and `n` / `N` still work) |
| `wrap` | `false` | long diff lines continue on the next screen rows instead of being cut off (see below) |

#### `wrap`

With `wrap: true`, a diff line longer than the diff pane continues on the next
screen rows instead of being cut off at the pane's edge. Continuation rows have
blank line numbers with `↪` in the gutter and keep the line's added or removed
colour. Lines break at the pane's edge,
never inside a wide character. `j`/`k`, counts, `G`, visual selection and
comments still move by diff lines, and `z h` / `z l` / `z H` / `z L` do nothing
while wrapping is on. Only the diff pane wraps.

A line taller than the whole pane (a minified file, say) shows only as much of
itself as fits; the rest cannot be scrolled to while wrapping is on. A pane too
narrow to hold a wide character (a code area of one cell) does not wrap.

### `tabs`

A non-empty list. Tab names must be unique.

| Key | Required | Meaning |
|---|---|---|
| `name` | yes | Label shown on the tab |
| `user` | no | GitHub login the sections are about. Default `@me`, the signed-in user |
| `sections` | no | The tab's sections. Default: the default sections below |

### `sections`

Each section is one PR list driven by a GitHub search query.

| Key | Required | Meaning |
|---|---|---|
| `name` | yes | Section heading |
| `query` | yes | GitHub search query. `{user}` is replaced by the tab's `user` |
| `limit` | no | Most PRs to fetch, 1 to 100. Default 30 |
| `sort` | no | `updated` (default), `created`, `comments`, `reactions` or `interactions` |
| `order` | no | `desc` (default) or `asc` |

`{user}` is the only variable; any other `{...}` in a query is an error.

Queries list pull requests only. If a query has no `is:pr` (or `type:pr`),
strig adds `is:pr` when it loads the file. A query must not contain a `sort:`
qualifier (an error); use the `sort` and `order` keys instead.

## Defaults

With no config file, or a file without `tabs`, strig has one tab, **Me**, with
these sections (`{user}` is `@me`):

| Section | Query |
|---|---|
| Review requested | `is:pr is:open review-requested:{user}` |
| My PRs | `is:pr is:open author:{user}` |
| Involved | `is:pr is:open involves:{user} -author:{user}` |

**Tabs you list replace the default Me tab.** If `tabs` has only a teammate,
you won't get your own dashboard. To keep it, list it first: `- name: Me`.

A tab with no `sections` gets the same three, so a teammate's tab is just
`- name: Alice` plus `user: alice`.
