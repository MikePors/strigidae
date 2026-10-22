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
