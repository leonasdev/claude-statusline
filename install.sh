#!/bin/bash
# Build and link the statusline into Claude Code.
# Usage: ./install.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$HOME/.claude/bin"

command -v go >/dev/null 2>&1 || { echo "go not found in PATH"; exit 1; }

echo "Building..."
cd "$SCRIPT_DIR"
go build -o statusline .

if [ "$SCRIPT_DIR" != "$TARGET" ]; then
    mkdir -p "$TARGET"
    cp statusline statusline.sh "$TARGET/"
    chmod +x "$TARGET/statusline.sh"
    echo "Installed to $TARGET"
else
    echo "Already at $TARGET"
fi

cat <<'EOF'

Add to ~/.claude/settings.json:

  "statusLine": {
    "type": "command",
    "command": "~/.claude/bin/statusline.sh",
    "refreshInterval": 1
  }

EOF
