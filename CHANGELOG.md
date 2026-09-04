# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.2.0] - 2026-09-04

### Added

- **Waterfall** view: a live rolling matrix, one row per busiest model
  (gradient-shaded by intensity) plus fixed "Agent" (Claude Code
  subagent/Task-tool turns, via the transcript's `isSidechain` field) and
  "Background" (poll-collector usage) rows.
- **Claude account status**: the Dashboard now shows Claude Code's login
  type (Claude.ai subscription — Pro/Max/Team — vs. a pay-as-you-go console
  API key) and, for a subscription, the rolling 5-hour session and 7-day
  weekly usage allowance, including "LOW PRIORITY" once the session window
  is spent. Read from Claude Code's own local cache; every reading is
  tagged with how stale it is (that cache refreshes on the CLI's own
  schedule, not per-request, so it can lag `/status` by a long way).
  Surfaced in both a Dashboard top bar and the Rate limits box (which no
  longer sits empty for "logs" users). Toggle in Settings ▸ Claude.
- Optional Nerd Font icon set for the tab bar and status bar (Settings ▸
  Appearance ▸ Nerd Font icons), off by default.

### Fixed

- Two side-by-side Dashboard panels (graph + rate limits) could render at
  different heights, so the shorter one's border closed early while the
  taller one kept going — read as a broken/disconnected border. Both now
  share one explicit height.
- The dashboard/session mini-graphs' resampler zero-padded when stretching
  fewer samples across more pixel columns than data points, bunching real
  data at one edge instead of spreading it proportionally across the window.
- Settings' hint column could overflow the panel's right border and wrap at
  the terminal edge instead, breaking out of the box visually; hints are now
  truncated to fit.
- The Help view had no way to scroll, so content past the terminal height
  was simply unreachable; it now scrolls independently (`↑/↓`, `pgup/pgdn`).

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
