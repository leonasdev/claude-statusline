# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A self-maintained custom statusline for Claude Code, written in Go. Replaces the third-party `ccstatusline`. Invoked once per CC tick (~1s) via `statusline.sh`, which auto-rebuilds the binary when `statusline.go` is newer than `statusline`.

Wired into CC via `~/.claude/settings.json`:
```json
"statusLine": { "type": "command", "command": "~/personal/claude-statusline/statusline.sh", "refreshInterval": 1 }
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

# Capture the actual stdin CC is sending right now (one-shot, from the repo dir)
touch .dump-stdin && sleep 1.5 && jq . cc-stdin.json
```

Build failures are captured in `build.log` next to the script and do NOT overwrite the existing binary — the previous version keeps running so CC's status bar never breaks.

## Architecture

Single Go file (`statusline.go`) partitioned by `// ==== SECTION: NAME ====` comments. One segment per renderer function. Output is `{path} │ {git?} │ {model} │ {effort} │ {ctx} │ {session} │ {reset}`, joined by a dimmed ` │ ` separator; empty segments drop out cleanly.

### Data flow per tick
1. Parse CC's stdin JSON (`Input` struct mirrors the schema).
2. Render each segment as a pure function of fields read directly from stdin — `Effort.Level`, `Model.DisplayName`, `Workspace.CurrentDir`, `ContextWindow.UsedPercentage`, `RateLimits.FiveHour.*`.
3. Print joined output. No state is persisted between ticks.

CC's stdin (since 2.1.119) directly emits `effort.level` (already resolved — `auto` becomes the concrete value like `max`/`high`/`medium`), so there's no sidecar cache and no `~/.claude/settings.json` fallback. Earlier versions of this codebase had both; `git log -- statusline.go` shows the simplification commit. The one transcript read is the ultracode disambiguation below.

### Effort display rules
- `effort.level` comes pre-resolved from CC stdin. Map keys: `low`, `medium`, `high`, `xhigh`, `max`, `ultracode`. Unknown values fall back to `high` icon.
- CC never emits `ultracode` in `effort.level` — the stdin enum stops at the five levels above, and ultracode sessions report `xhigh` (ultracode is a session-scoped flag held in CC process memory; not in settings.json, not in any env var). When stdin says `xhigh`, `resolveEffortLevel` reads the last 4 MiB of `transcript_path` and takes the most recent `/effort` output entry (`<local-command-stdout>Set effort level to …`). Genuine entries are type-`user` with plain-string content; quoted copies inside tool results have array content and don't match. Resume staleness is handled by `ultracodeMarkerStale`: a marker timestamped before the owning `claude` process started (parent-tree walk via `readPPID`; start = `/proc/stat` btime + `/proc/<pid>/stat` starttime at USER_HZ 100) is ignored — session-scoped ultracode doesn't survive a CC restart.
- The transcript fallback is temporary scaffolding. A future CC version will likely emit ultracode (or a workflow flag) in stdin directly; `resolveEffortLevel` already passes a literal `ultracode` level straight through, so when that happens the fallback (`resolveEffortLevel` transcript scan, `ultracodeMarkerStale`, `ccStartUnix`/`processStartUnix`/`parseStarttimeTicks`/`parseBtime`, `readFileTail`) can be deleted wholesale — the icon map alone suffices. After a CC update, `touch .dump-stdin` and check the JSON for ultracode/workflow fields.
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

The porcelain call is bounded by `gitStatusTimeout` (1s) via `exec.CommandContext`: `-uall` enumerates every untracked file, so a repo with a huge untracked tree (un-ignored build/data dir) takes seconds and would block the whole render. On timeout `runGitInfo` returns `statusOK=false`, and the segment shows `{branch} …` (`gitStatusUnknownMarker`) instead of the dirty counts — branch is fetched first so it survives. The SIGKILL on timeout is safe: `--no-optional-locks` status is read-only and takes no `index.lock`.

### Color handling
Path segment is wrapped with a full `\x1b[0m` reset (not just `\x1b[39m`) so CC's TUI ambient attributes (dim, bold) don't bleed into it. Don't "simplify" that to a plain foreground reset.

## Testing conventions

- All tests are in `statusline_test.go`; subtests group by section.
- All tests are pure-function unit tests (no fixtures, no external state). Adding a new segment? Add a `TestRender<Name>Segment` test and any pure helpers it depends on.

## Capturing real CC stdin

From the repo dir, `touch .dump-stdin` and the next tick (~1s) writes that tick's stdin to `cc-stdin.json` and removes the sentinel.

