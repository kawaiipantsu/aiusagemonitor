#!/usr/bin/env bash
# Builds one Debian package for a given Go arch / Debian arch pair.
# Usage: build-deb.sh <goarch> <debarch> <version> <binary> <ldflags> <distdir>
set -euo pipefail

GOARCH_IN="$1"
DEBARCH="$2"
VERSION="$3"
BINARY="$4"
LDFLAGS="$5"
DIST="$6"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GOARM=""
if [ "$GOARCH_IN" = "arm" ]; then
  GOARM=7
fi

if ! command -v fakeroot >/dev/null 2>&1 || ! command -v dpkg-deb >/dev/null 2>&1; then
  echo "build-deb.sh: needs 'fakeroot' and 'dpkg-deb' (Debian/Ubuntu: apt-get install fakeroot dpkg-dev)" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
PKGROOT="$WORK/pkg"
mkdir -p "$PKGROOT/DEBIAN" "$PKGROOT/usr/bin" "$PKGROOT/usr/share/man/man1" "$PKGROOT/usr/share/doc/$BINARY"

echo "==> building $BINARY for linux/$GOARCH_IN (deb arch: $DEBARCH)"
# NOTE: must be a literal `GOARM=...` word (even when empty) so bash's parser
# recognizes it as an assignment prefix at parse time. A parameter expansion
# like ${GOARM:+GOARM=$GOARM} is *not* recognized as an assignment (bash
# decides that from the unexpanded source text), so it was instead taken as
# the command name itself, shifting `go build ...` into its arguments and
# failing with "GOARM=7: command not found".
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH_IN" GOARM="$GOARM" \
  go build -trimpath -ldflags "$LDFLAGS" -o "$PKGROOT/usr/bin/$BINARY" ./cmd/aiusagemonitor

gzip -9 -n -c "packaging/man/$BINARY.1" > "$PKGROOT/usr/share/man/man1/$BINARY.1.gz"
cp "packaging/debian/copyright" "$PKGROOT/usr/share/doc/$BINARY/copyright"
if [ -f CHANGELOG.md ]; then
  gzip -9 -n -c CHANGELOG.md > "$PKGROOT/usr/share/doc/$BINARY/changelog.gz"
fi

# Debian version strings must start with a digit.
DEBVER="${VERSION#v}"
case "$DEBVER" in
  [0-9]*) ;;
  *) DEBVER="0.0.0~${DEBVER}" ;;
esac

# NOTE: use printf, not echo -- piping echo's trailing newline through
# `tr -c` turns that newline into a literal trailing '-' (it's not in the
# allowed set either), which command substitution then can't strip because
# the output no longer *ends* with a newline. That produced invalid
# "revision number is empty" versions like "0.0.0~dev-".
DEBVER="$(printf '%s' "$DEBVER" | tr -c 'A-Za-z0-9.+~-' '-')"

SIZE_KB="$(du -sk "$PKGROOT" | cut -f1)"
cat > "$PKGROOT/DEBIAN/control" <<EOF
Package: $BINARY
Version: $DEBVER
Section: utils
Priority: optional
Architecture: $DEBARCH
Installed-Size: $SIZE_KB
Maintainer: kawaiipantsu <12233528+kawaiipantsu@users.noreply.github.com>
Homepage: https://thugs.red
Description: Live TUI usage/limit monitor for AI vendors
 aiusagemonitor is a terminal UI that shows live token usage, rate limits,
 cost and historic statistics for OpenAI, Claude (Anthropic), Google Gemini
 and xAI. It gathers data by tailing local AI-CLI session logs (Claude Code,
 Codex CLI, Gemini CLI), through a built-in local capture proxy, or by
 polling a vendor's usage API, and keeps a local SQLite history for
 summaries and usage profiling.
EOF

mkdir -p "$DIST"
OUT="$DIST/${BINARY}_${DEBVER}_${DEBARCH}.deb"
fakeroot dpkg-deb --build --root-owner-group "$PKGROOT" "$OUT" >/dev/null
echo "-> $OUT"
