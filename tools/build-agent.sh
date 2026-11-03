#!/usr/bin/env bash
# Builds raisesteady-guard-agent.exe for windows/amd64 and windows/arm64 into dist/.
# Copies the shared data and the design tokens into the module (for go:embed),
# then runs go vet (host and windows) and go test unless --no-test is given.
# Needs Go (brew install go).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
AG="$ROOT/agent"
DATA="$AG/internal/core/data"
WEB="$AG/internal/api/web"
DIST="$ROOT/dist"
RUN_TESTS=1
[ "${1:-}" = "--no-test" ] && RUN_TESTS=0

command -v go >/dev/null || { echo "Go is not installed (brew install go)"; exit 1; }
mkdir -p "$DATA" "$DIST"

# Shared data: age bands are required; the rest may be missing (built empty, said so).
[ -f "$ROOT/shared/age-bands.json" ] || { echo "missing shared/age-bands.json"; exit 1; }
for f in age-bands.json bookmarks.json ext-ids.json unsupported-browsers.json; do
  if [ -f "$ROOT/shared/$f" ]; then
    cp "$ROOT/shared/$f" "$DATA/$f"
  else
    echo "note: shared/$f is missing; building without it"
    : > "$DATA/$f"
  fi
done
# The setup command (Linux) shows the same terms as the Windows installer.
for l in en ka ru; do
  [ -f "$ROOT/installer/terms-$l.txt" ] || { echo "missing installer/terms-$l.txt"; exit 1; }
  cp "$ROOT/installer/terms-$l.txt" "$DATA/terms-$l.txt"
done
[ -f "$ROOT/ext/ui/tokens.css" ] || { echo "missing ext/ui/tokens.css"; exit 1; }
cp "$ROOT/ext/ui/tokens.css" "$WEB/tokens.css"

VERSION="${KG_VERSION:-0.2.2}"
if git -C "$ROOT" rev-parse --short HEAD >/dev/null 2>&1; then
  VERSION="$VERSION+$(git -C "$ROOT" rev-parse --short HEAD)"
fi

cd "$AG"
export CGO_ENABLED=0 GOFLAGS=-mod=readonly
go mod verify >/dev/null
if [ "$RUN_TESTS" = 1 ]; then
  go vet ./...
  GOOS=windows GOARCH=amd64 go vet ./...
  GOOS=windows GOARCH=arm64 go vet ./...
  go test -count=1 ./...
fi
# File details (product name, version, icon) for Windows and for code signing
# (SignPath asks for the product name and version in every signed file).
# go-winres v0.3.3 (ISC-style licence, a build tool, not shipped) writes
# rsrc_windows_<arch>.syso, which go build links in. It runs where KG_WINRES=1
# (the GitHub build); local builds skip it and have no file details.
rm -f rsrc_windows_*.syso
if [ "${KG_WINRES:-0}" = 1 ]; then
  NAME="$(sed -n 's/^const AppName = "\(.*\)"$/\1/p' internal/core/core.go)"
  NUM="${VERSION%%+*}.0"
  GOFLAGS= go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --manifest cli \
    --product-name "$NAME" --file-description "$NAME service" \
    --product-version "$NUM" --file-version "$NUM" --original-filename raisesteady-guard-agent.exe \
    --copyright "Nexodyne Ltd. GPL-3.0" --icon "$ROOT/ext/icons/icon-128.png"
fi
for arch in amd64 arm64; do
  GOOS=windows GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "$DIST/raisesteady-guard-agent-$arch.exe" .
done
ls -l "$DIST"/raisesteady-guard-agent-*.exe
