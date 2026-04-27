#!/bin/bash
SRC="$HOME/.claude/bin/statusline.go"
BIN="$HOME/.claude/bin/statusline"
if [ "$SRC" -nt "$BIN" ]; then
    cd "$HOME/.claude/bin" && go build -o "$BIN" . 2>/tmp/claude-statusline-build.log
fi
if [ -n "$STATUSLINE_DUMP" ]; then
    exec tee "$STATUSLINE_DUMP" | "$BIN"
else
    exec "$BIN"
fi
