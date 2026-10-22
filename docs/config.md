# Configuration

strig reads a YAML file at `$XDG_CONFIG_HOME/strig/config.yml` (falling back to
`~/.config/strig/config.yml`). `strig --config <path>` uses another file, and
`strig --config-path` prints the location that would be used.

A missing file is fine: strig uses the defaults below. A file named with
`--config` must exist. A file with a mistake stops strig before the screen
starts, with the file, line and problem, for example:

    strig: /home/me/.config/strig/config.yml:7: unknown key "limt" in a section (valid keys: name, query, limit, sort, order)

Unknown keys are errors so typos don't pass silently. A commented example is in
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

## Defaults

With no config file, or a file without `tabs`, strig has one tab, **Me**, with
these sections (`{user}` is `@me`):

| Section | Query |
|---|---|
| Review requested | `is:pr is:open review-requested:{user}` |
| My PRs | `is:pr is:open author:{user}` |
| Involved | `is:pr is:open involves:{user} -author:{user}` |

A tab with no `sections` gets the same three, so a teammate's tab is just
`- name: Alice` plus `user: alice`.
