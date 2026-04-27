#!/bin/bash
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$DIR/statusline.go"
BIN="$DIR/statusline"
SENTINEL="$DIR/.dump-stdin"
DUMP="$DIR/cc-stdin.json"
if [ -f "$SRC" ] && [ "$SRC" -nt "$BIN" ]; then
    cd "$DIR" && go build -o "$BIN" . 2>"$DIR/build.log"
fi
if [ -e "$SENTINEL" ]; then
    rm -f "$SENTINEL"
    exec tee "$DUMP" | "$BIN"
else
    exec "$BIN"
fi
