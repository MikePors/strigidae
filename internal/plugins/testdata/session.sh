#!/bin/sh
# Prints its process number and its process group number as a notice. pgid is
# the portable ps keyword (sid is not in POSIX and macOS ps rejects it).
group=$(ps -o pgid= -p $$ 2>/dev/null | tr -d ' ')
printf '{"name":"notice","data":{"text":"%s %s"}}\n' "$$" "$group"
