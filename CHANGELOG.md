# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.0] - 2026-09-04

### Added

- Initial release: a Go TUI (Bubble Tea) for live token usage, rate limits,
  cost and historic statistics across OpenAI, Claude (Anthropic), Google
  Gemini and xAI.
- Pluggable collectors per provider: local AI-CLI log tailing (Claude Code,
  Codex CLI, Gemini CLI — experimental), a built-in local capture proxy that
  records real request/response usage and rate-limit headers, and usage-API
  polling (OpenAI, Anthropic) for admin-scoped keys.
- Dashboard, Sessions, History, Profile, Settings and Help views; resizable,
  keyboard-driven, ten built-in themes.
- Local SQLite history (pure-Go driver, `CGO_ENABLED=0`) with automatic
  retention pruning, feeding the History and Profile (usage-profiling,
  activity heatmap, burn-rate projection) views.
- `report` and `proxy` headless subcommands for scripting/cron use.
- Cross-platform Makefile (Linux/Windows/macOS, amd64/386/arm64/armv7) and
  Debian packaging via `dpkg-deb`/`fakeroot` for amd64, i386, arm64 and armhf.
