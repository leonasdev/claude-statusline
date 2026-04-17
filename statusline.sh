#!/bin/bash
SRC="$HOME/.claude/bin/statusline.go"
BIN="$HOME/.claude/bin/statusline"
if [ "$SRC" -nt "$BIN" ]; then
    cd "$HOME/.claude/bin" && go build -o "$BIN" . 2>/tmp/claude-statusline-build.log
fi
exec "$BIN"
