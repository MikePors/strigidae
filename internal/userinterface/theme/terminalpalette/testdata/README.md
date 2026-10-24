Terminal answers to strig's colour queries, for the parser tests. They are
written by hand in the format xterm, WezTerm and Ghostty use (OSC 10/11/4
answers with `rgb:` colours, then a DA1 answer), not captured from a terminal.
The colours are Tokyo Night's.

- full-st.reply: every colour, four hex digits per channel, ST-terminated.
- full-bel.reply: every colour, two hex digits per channel, BEL-terminated.
- partial.reply: foreground and background only, then DA1.
- da1-only.reply: a terminal that ignores the colour queries.
- none.reply: no answer at all.
