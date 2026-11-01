#!/bin/sh
# One run at a time, checkable: takes a lock directory, records its input in
# $FAKE_DIR/order and its start in $FAKE_DIR/started, then waits for
# $FAKE_DIR/gate to exist and a little longer, so a run that starts alongside it
# finds the lock taken and writes $FAKE_DIR/overlap.
if ! mkdir "$FAKE_DIR/lock" 2>/dev/null; then
  echo overlap >> "$FAKE_DIR/overlap"
fi
cat >> "$FAKE_DIR/order"
echo >> "$FAKE_DIR/order"
echo started >> "$FAKE_DIR/started"  # a fifo the test reads
until [ -e "$FAKE_DIR/gate" ]; do sleep 0.02; done
sleep 0.2
rmdir "$FAKE_DIR/lock"
echo '{"name":"notice","data":{"text":"ok"}}'
