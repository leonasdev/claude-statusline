# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A self-maintained custom statusline for Claude Code, written in Go. Replaces the third-party `ccstatusline`. Invoked once per CC tick (~1s) via `statusline.sh`, which auto-rebuilds the binary when `statusline.go` is newer than `statusline`.

Wired into CC via `~/.claude/settings.json`:
```json
"statusLine": { "type": "command", "command": "~/.claude/bin/statusline.sh", "refreshInterval": 1 }
```

## Commands

```bash
# Build (also happens automatically via statusline.sh)
go build -o statusline .

# Run tests (fast — all pure functions, no fixtures needed)
go test ./...
go test -run TestParseInput ./...   # single test
go test -v ./...                    # verbose

# Manual end-to-end smoke test
echo '{"model":{"display_name":"Opus 4.7 (1M context)"},"workspace":{"current_dir":"/tmp"},"context_window":{"used_percentage":8},"rate_limits":{"five_hour":{"used_percentage":10,"resets_at":0}},"effort":{"level":"max"}}' | ./statusline

# Capture the actual stdin CC is sending right now (one-shot)
touch /tmp/.cc-dump-stdin && sleep 1.5 && cat /tmp/cc-stdin.json | jq .
```

Build failures are captured in `/tmp/claude-statusline-build.log` and do NOT overwrite the existing binary — the previous version keeps running so CC's status bar never breaks.

## Architecture

Single Go file (`statusline.go`) partitioned by `// ==== SECTION: NAME ====` comments. One segment per renderer function. Output is `{path} │ {git?} │ {model} │ {effort} │ {ctx} │ {session} │ {reset}`, joined by a dimmed ` │ ` separator; empty segments drop out cleanly.

### Data flow per tick
1. Parse CC's stdin JSON (`Input` struct mirrors the schema).
2. Render each segment as a pure function of fields read directly from stdin — `Effort.Level`, `Model.DisplayName`, `Workspace.CurrentDir`, `ContextWindow.UsedPercentage`, `RateLimits.FiveHour.*`.
3. Print joined output. No state is persisted between ticks.

CC's stdin (since v2.1.x) directly emits `effort.level` (already resolved — `auto` becomes the concrete value like `max`/`high`/`medium`), so there's no transcript scanning, no sidecar cache, and no `~/.claude/settings.json` fallback. Earlier versions of this codebase had all three; `git log -- statusline.go` shows the simplification commit.

### Effort display rules
- `effort.level` comes pre-resolved from CC stdin. Map keys: `low`, `medium`, `high`, `xhigh`, `max`. Unknown values fall back to `high` icon.
- Haiku models hide the effort segment entirely (`modelHasNoEffort`).

### Path abbreviation (`fitPath`)
5 levels, pick the first whose rendered length ≤ `pathThreshold` (40 runes, NOT terminal-width-derived — a deliberately fixed budget picked for readability):
- L0: full
- L1: first non-anchor parent → 1 char (`~/p/test-projects/project-1`)
- L2: fish-style, all parents → 1 char
- L3: `…/leaf`
- L4: truncate leaf

### Git segment
Runs `git branch --show-current` + `git status --porcelain=v1 -uall` in the CWD. Falls back to short SHA for detached HEAD. Rename/copy (`R`/`C`) porcelain entries are ignored by design. Not cached — `git status` is already <10ms on small repos and caching would lag visible state.

### Color handling
Path segment is wrapped with a full `\x1b[0m` reset (not just `\x1b[39m`) so CC's TUI ambient attributes (dim, bold) don't bleed into it. Don't "simplify" that to a plain foreground reset.

## Testing conventions

- All tests are in `statusline_test.go`; subtests group by section.
- All tests are pure-function unit tests (no fixtures, no external state). Adding a new segment? Add a `TestRender<Name>Segment` test and any pure helpers it depends on.

## Capturing real CC stdin

`statusline.sh` supports two debug modes for inspecting what CC actually pipes to the binary:
- One-shot: `touch /tmp/.cc-dump-stdin` — next tick (~1s) writes stdin to `/tmp/cc-stdin.json` and auto-removes the sentinel. Read it with `cat /tmp/cc-stdin.json | jq .`.
- Continuous: set `STATUSLINE_DUMP=/path/to/file` in the env CC launches the script with — every tick overwrites the file.

