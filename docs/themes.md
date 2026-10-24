# Themes

strig draws with a built-in theme that follows your terminal: dark or light,
whichever the terminal reports. To use other colours, name a bundled theme or
a Base16 or Base24 scheme file in the config:

```yaml
theme: tokyo-night            # a bundled theme
# theme: ~/themes/gruvbox.yaml   # or any Base16/Base24 scheme file
```

`strig --list-themes` prints the bundled themes, one per line. Currently:
`catppuccin-latte` (light) and `tokyo-night`.

A bundled theme or theme file also sets the terminal's own background and
foreground to the theme's `background` and `text` colours while strig runs, so
a dark theme looks right on a light terminal. The terminal restores its colours
when strig exits. The built-in theme leaves them alone.

Coming later: `syntax:` (named syntax styles, #82), `:theme` to switch themes
while strig runs (#83), and `theme: terminal` and Xresources files, which build
a theme from the terminal's own palette (#84). Until then `theme: terminal`
is an error saying it is not supported yet, and `syntax:` in a theme file is
accepted but ignored (strig logs a warning; syntax colours come from the
palette).

## The three layers

Colour flows through three layers, each built only from the one below.

1. **Palette.** The 24 colours of a [Base24](https://github.com/tinted-theming/base24)
   scheme, `base00` to `base17`, plus whether the scheme is dark or light. A
   Base16 scheme (`base00` to `base0F`) works too: strig fills the missing eight
   colours as the Base16 documentation describes. This is the only place hex
   colours appear in strig's code.
2. **Roles.** What strig means by a colour: `text`, `muted`, `accent`, `added`,
   the diff tints, the syntax colours. Each role is derived from palette
   colours by a fixed rule (the table below). A theme file can replace any
   role with an exact colour.
3. **Styles.** The named styles the screens draw with, built from roles only.

Derived foreground roles go through a contrast check against the colours they
are drawn on, and are nudged toward black or white if they would be hard to
read. A colour you set in `roles:` is used exactly as written; if it is hard to
read, strig only logs a warning.

## Roles

`base0X` below are palette colours. Where a role is a tint, it is made by
mixing the named role into `background` (in OKLCH, so the hue and saturation are
kept), at fixed strengths that differ for dark and light themes.

| Role | Default | Where it shows |
|---|---|---|
| `background` | `base00` | Base of every tint; the terminal background for a chosen theme |
| `surface` | `base01` | Status bar, command line, code spans |
| `text` | `base05` | Body text; the terminal foreground for a chosen theme |
| `muted` | `base03` | De-emphasised text, line numbers, hints, draft state |
| `border` | `muted` | Pane borders. Follows `muted` unless set |
| `selection` | `base02` | The cursor row in the diff |
| `accent` | `base0E` | Focus, headings, the focused pane border and title, merged state, tabs |
| `accent-strong` | `base0E` mixed 40% toward `base07` | The selected item in lists and popups |
| `on-accent` | `base00` | Text drawn on an `accent` background |
| `info` | `base0D` | Hunk headers, thread bars |
| `link` | `base0C` | Hyperlinks |
| `warning` | `base09` | Pending reviews, numbers in lists, key hints, modified files |
| `error` | `base08` | Failures, changes requested, closed state |
| `success` | `base0B` | Success messages, open state, approvals |
| `added` | `base0B` | Added lines and files, passing checks |
| `removed` | `base08` | Deleted lines and files, failing checks |
| `added-tint` | `added` into `background` | Background of added diff lines |
| `added-gutter-tint` | `added` into `background`, stronger | Line-number background of added lines |
| `removed-tint` | `removed` into `background` | Background of deleted diff lines |
| `removed-gutter-tint` | `removed` into `background`, stronger | Line-number background of deleted lines |
| `hunk-tint` | `info` into `background` | Hunk header rows |
| `hunk-cursor-tint` | `info` into `background`, stronger | Hunk header under the cursor |
| `syn-keyword` | `base0E` | Keywords |
| `syn-string` | `base0B` | Strings |
| `syn-comment` | `base03` | Comments |
| `syn-number` | `base09` | Numbers and constants |
| `syn-function` | `base0D` | Functions, built-ins, decorators |
| `syn-type` | `base0A` | Types, classes, namespaces |
| `syn-operator` | `base0C` | Operators and preprocessor lines |
| `syn-variable` | `base08` | Variables and tags |
| `syn-escape` | `base0C` | String escapes and regular expressions |

The syntax roles colour code in the diff and in Markdown code blocks alike.
Overriding `added` or `removed` also recolours the tints derived from them,
unless you set the tints too.

## Using a Base16 or Base24 scheme

Any scheme from the [tinted-theming schemes](https://github.com/tinted-theming/schemes)
repository works unchanged: download the YAML file and point `theme:` at it.

```yaml
theme: ~/themes/gruvbox-dark-hard.yaml
```

Both file layouts are accepted.

The current layout, with `system` and a `palette` table:

```yaml
system: "base24"
name: "Tokyo Night Dark"
author: "Michael Ball"
variant: "dark"
palette:
  base00: "#1a1b26"
  # ... base01 to base17
```

The older flat layout, with a `scheme` name and the colours at the top level,
usually without a `#`:

```yaml
scheme: "3024"
author: "Jan T. Sott"
base00: "090300"
# ... base01 to base0F
```

Accepted keys, and nothing else (any other key is an error naming the file, the
line and the key):

- current layout: `system`, `name`, `slug`, `author`, `description`, `variant`,
  `palette`;
- flat layout: `scheme`, `author`, `variant`, and `base00` to `base0F`
  (Base16) or `base00` to `base17` (Base24);
- both layouts: `syntax` and `roles` (strig's extensions, below).

Every colour is six hex digits, with or without a leading `#`. Quote a colour
that starts with `#`, or YAML reads it as a comment.

`variant` is `dark` or `light`. When the file has none (the flat layout never
does) strig decides from `base00`: dark if white text reads better on it than
black.

## Overrides

`roles:` sets exact colours for any role in the table above, including the
tints and `border`:

```yaml
roles:
  muted: "#565f89"
  border: "#3b4261"
  added-tint: "#20303b"
```

A role name that is not in the table is an error that lists the valid ones.
Use this where a scheme's slot does not hold the colour your editor theme uses
for that purpose. For example, many schemes put a surface colour in `base03`,
which is too faint for comments and borders.

`syntax:` names a syntax style for code. It is accepted, but ignored for now
(#82).

## How `theme:` is resolved

| Value | Meaning |
|---|---|
| unset | The built-in theme, dark or light to match the terminal |
| contains `/`, or ends in `.yaml` or `.yml` | A theme file. `~` is expanded to your home directory, and a relative path is taken from the directory of the config file |
| `terminal` | Reserved; not supported yet (#84) |
| anything else | A bundled theme's name, lower-case and hyphenated, as `strig --list-themes` prints it |

A theme that cannot be found or read stops strig before the screen starts,
with the file or name and the problem.

## Bundled themes

| Name | Variant | Source |
|---|---|---|
| `tokyo-night` | dark | Palette from the Base24 scheme "Tokyo Night Dark" in tinted-theming/schemes (MIT), based on Tokyo Night by enkia (MIT). Role overrides use the colours of Tokyo Night's night palette as used by folke/tokyonight.nvim (Apache-2.0) for text, comments, borders, selection, status colours, diff tints and syntax |
| `catppuccin-latte` | light | Palette from the Base24 scheme "Catppuccin Latte" in tinted-theming/schemes (MIT), based on Catppuccin (MIT). Muted text and borders use Latte's own subtext and overlay colours |

Each theme file under `internal/userinterface/theme/scheme/bundled/` records
its source and licence in its header.
