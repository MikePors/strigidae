#!/bin/sh
# Starts a child that keeps standard output open, records its pid in
# $FAKE_DIR/pid, prints an effect and exits 0.
sleep 300 &
echo $! > "$FAKE_DIR/pid"
echo '{"name":"notice","data":{"text":"done"}}'
exit 0
