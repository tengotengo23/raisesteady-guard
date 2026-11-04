#!/usr/bin/env bash
# Builds RaiseSteady Guard for Linux into dist/linux/: the program for amd64
# and arm64 (static, runs on any systemd Linux), a .deb and an .rpm for each
# (nFPM), and a .tar.gz with install.sh for the rest.
#   tools/build-linux.sh [--no-test] [--no-packages]
# nFPM: set NFPM to its path, or have it on PATH
# (go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
AG="$ROOT/agent"
DATA="$AG/internal/core/data"
WEB="$AG/internal/api/web"
OUT="$ROOT/dist/linux"
RUN_TESTS=1
PACKAGES=1
for a in "$@"; do
  case "$a" in
    --no-test) RUN_TESTS=0 ;;
    --no-packages) PACKAGES=0 ;;
    *) echo "unknown option $a"; exit 2 ;;
  esac
done

command -v go >/dev/null || { echo "Go is not installed"; exit 1; }
mkdir -p "$DATA" "$OUT"
for f in age-bands.json bookmarks.json ext-ids.json unsupported-browsers.json pc-apps.json; do
  cp "$ROOT/shared/$f" "$DATA/$f"
done
for l in en ka ru; do cp "$ROOT/installer/terms-$l.txt" "$DATA/terms-$l.txt"; done
cp "$ROOT/ext/ui/tokens.css" "$WEB/tokens.css"

VERSION="${KG_VERSION:-0.3.2}"   # package version: digits and dots only
PV="$VERSION"                     # program version: with the commit
if git -C "$ROOT" rev-parse --short HEAD >/dev/null 2>&1; then
  PV="$VERSION+$(git -C "$ROOT" rev-parse --short HEAD)"
fi

cd "$AG"
export CGO_ENABLED=0 GOFLAGS=-mod=readonly
go mod verify >/dev/null
if [ "$RUN_TESTS" = 1 ]; then
  go vet ./...
  GOOS=linux GOARCH=amd64 go vet ./...
  GOOS=linux GOARCH=arm64 go vet ./...
  go test -count=1 ./...
fi
for arch in amd64 arm64; do
  GOOS=linux GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$PV" \
    -o "$OUT/raisesteady-guard-linux-$arch" .
done
echo "built $OUT/raisesteady-guard-linux-{amd64,arm64} ($PV)"

[ "$PACKAGES" = 1 ] || exit 0
NFPM="${NFPM:-nfpm}"
command -v "$NFPM" >/dev/null || { echo "nFPM not found: set NFPM or install v2.47.0"; exit 1; }

XPI=""
# The Firefox add-on signed by Mozilla (signed/README.md), unless the build already made one.
[ ! -f "$ROOT/dist/raisesteady-guard.xpi" ] && [ -f "$ROOT/signed/raisesteady-guard-firefox.xpi" ] && \
  mkdir -p "$ROOT/dist" && cp "$ROOT/signed/raisesteady-guard-firefox.xpi" "$ROOT/dist/raisesteady-guard.xpi"
[ -f "$ROOT/dist/raisesteady-guard.xpi" ] && XPI="$ROOT/dist/raisesteady-guard.xpi"
echo "Firefox add-on in the packages: ${XPI:-none}"
rm -f "$OUT"/*.deb "$OUT"/*.rpm "$OUT"/*.tar.gz
for arch in amd64 arm64; do
  cfg="$OUT/nfpm-$arch.yaml"
  {
    cat <<EOF
name: raisesteady-guard
arch: $arch
platform: linux
version: $VERSION
release: 1
section: admin
priority: optional
maintainer: "Tengiz Lominadze <33996692+tengotengo23@users.noreply.github.com>"
vendor: Nexodyne Limited
homepage: https://github.com/tengotengo23/raisesteady-guard
license: GPL-3.0-only
description: |-
  Safe browser settings for children on Linux.
  Sets safe search, a family DNS filter and no private windows in Chrome,
  Chromium, Edge, Brave and Firefox, and closes browsers it cannot protect.
  Made with AI and provided without any guarantee; support is voluntary.
overrides:
  deb:
    recommends: [xdg-utils, libnotify-bin]
  rpm:
    recommends: [xdg-utils, libnotify]
contents:
  - src: $OUT/raisesteady-guard-linux-$arch
    dst: /usr/lib/raisesteady-guard/raisesteady-guard
    file_info: {mode: 0755}
  - src: $ROOT/linux/open-parent-page
    dst: /usr/lib/raisesteady-guard/open-parent-page
    file_info: {mode: 0755}
  - src: /usr/lib/raisesteady-guard/raisesteady-guard
    dst: /usr/bin/raisesteady-guard
    type: symlink
  - src: $ROOT/linux/raisesteady-guard.service
    dst: /usr/lib/systemd/system/raisesteady-guard.service
    file_info: {mode: 0644}
  - src: $ROOT/linux/raisesteady-guard.desktop
    dst: /usr/share/applications/raisesteady-guard.desktop
    file_info: {mode: 0644}
  - src: $ROOT/ext/icons/icon-128.png
    dst: /usr/share/icons/hicolor/128x128/apps/raisesteady-guard.png
    file_info: {mode: 0644}
  - src: $ROOT/README.md
    dst: /usr/share/doc/raisesteady-guard/README.md
    file_info: {mode: 0644}
  - src: $ROOT/LICENSE
    dst: /usr/share/doc/raisesteady-guard/LICENSE
    file_info: {mode: 0644}
  - src: $ROOT/THIRD-PARTY-NOTICES.txt
    dst: /usr/share/doc/raisesteady-guard/THIRD-PARTY-NOTICES.txt
    file_info: {mode: 0644}
EOF
    for l in en ka ru; do
      printf '  - src: %s\n    dst: %s\n    file_info: {mode: 0644}\n' "$ROOT/installer/terms-$l.txt" "/usr/share/doc/raisesteady-guard/terms-$l.txt"
    done
    if [ -n "$XPI" ]; then
      printf '  - src: %s\n    dst: /usr/lib/raisesteady-guard/raisesteady-guard.xpi\n    file_info: {mode: 0644}\n' "$XPI"
    fi
    cat <<EOF
scripts:
  postinstall: $ROOT/linux/postinstall.sh
  preremove: $ROOT/linux/preremove.sh
  postremove: $ROOT/linux/postremove.sh
EOF
  } > "$cfg"
  "$NFPM" package --config "$cfg" --packager deb --target "$OUT/" >/dev/null
  "$NFPM" package --config "$cfg" --packager rpm --target "$OUT/" >/dev/null
  rm -f "$cfg"

  # The .tar.gz for other systemd Linux systems (install.sh).
  name="raisesteady-guard-$VERSION-linux-$arch"
  st="$OUT/$name"
  rm -rf "$st"
  mkdir -p "$st"
  cp "$OUT/raisesteady-guard-linux-$arch" "$st/raisesteady-guard"
  cp "$ROOT/linux/install.sh" "$ROOT/linux/uninstall.sh" "$ROOT/linux/open-parent-page" \
     "$ROOT/linux/raisesteady-guard.service" "$ROOT/linux/raisesteady-guard.desktop" \
     "$ROOT/README.md" "$ROOT/LICENSE" "$ROOT/THIRD-PARTY-NOTICES.txt" "$st/"
  cp "$ROOT/ext/icons/icon-128.png" "$st/raisesteady-guard.png"
  for l in en ka ru; do cp "$ROOT/installer/terms-$l.txt" "$st/"; done
  [ -n "$XPI" ] && cp "$XPI" "$st/raisesteady-guard.xpi"
  chmod 0755 "$st/raisesteady-guard" "$st/install.sh" "$st/uninstall.sh" "$st/open-parent-page"
  if tar --version 2>/dev/null | grep -q GNU; then
    tar -C "$OUT" --owner=0 --group=0 --numeric-owner -czf "$OUT/$name.tar.gz" "$name"
  else
    COPYFILE_DISABLE=1 tar -C "$OUT" --uid 0 --gid 0 -czf "$OUT/$name.tar.gz" "$name"
  fi
  rm -rf "$st"
done
ls -1 "$OUT"/*.deb "$OUT"/*.rpm "$OUT"/*.tar.gz
