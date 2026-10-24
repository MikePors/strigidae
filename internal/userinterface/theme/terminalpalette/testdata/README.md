Terminal answers to strig's colour queries, for the parser tests. They are
written by hand in the format xterm, WezTerm and Ghostty use (OSC 10/11/4
answers with `rgb:` colours, then a DA1 answer), not captured from a terminal.
The colours are Tokyo Night's.

- full-st.reply: every colour, four hex digits per channel, ST-terminated.
- full-bel.reply: every colour, BEL-terminated; the first two answers use four hex digits per channel, the ANSI answers two.
- partial.reply: foreground and background only, then DA1.
- da1-only.reply: a terminal that ignores the colour queries.
- none.reply: no answer at all.
- tmux-passthrough.reply: inside tmux with `allow-passthrough on`: tmux answers the foreground and background itself, then the outer terminal answers the 16 ANSI colours and DA1. It reads the same as full-st.reply.
- tmux-no-passthrough.reply: inside tmux with passthrough off: only tmux's own foreground and background answers; the wrapped queries are dropped, so no ANSI colours and no DA1 arrive.
