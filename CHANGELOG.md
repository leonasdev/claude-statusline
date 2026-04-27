# Changelog

All notable changes to this project will be documented in this file.
Format roughly follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added
- README, LICENSE (MIT), GitHub Actions CI, install script.

### Changed
- Read `effort.level` directly from CC stdin (CC v2.1.x+); removed the
  transcript reverse-scan, sidecar cache, and `settings.json` fallback.
  Net -729 lines, no segment-output behavior change.
- `statusline.sh` gained a stdin-dump debug mode
  (`touch /tmp/.cc-dump-stdin` for one-shot, `STATUSLINE_DUMP=path` for
  continuous).
