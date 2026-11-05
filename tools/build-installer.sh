#!/usr/bin/env bash
# Builds dist/RaiseSteadyGuardSetup.exe with NSIS (brew install makensis, or
# NSIS on Windows). Runs build-agent.sh first unless the agents are there and
# --skip-agent is given. Includes dist/raisesteady-guard.xpi when it exists.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
VERSION="${KG_VERSION:-0.4.0}"

if [ "${1:-}" != "--skip-agent" ] || [ ! -f "$DIST/raisesteady-guard-agent-amd64.exe" ]; then
  "$ROOT/tools/build-agent.sh" ${KG_AGENT_FLAGS:-}
fi

MAKENSIS="$(command -v makensis || true)"
if [ -z "$MAKENSIS" ] && [ -x "/c/Program Files (x86)/NSIS/makensis.exe" ]; then
  MAKENSIS="/c/Program Files (x86)/NSIS/makensis.exe"
fi
[ -n "$MAKENSIS" ] || { echo "NSIS is not installed (brew install makensis)"; exit 1; }

winpath() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
# The Firefox add-on signed by Mozilla (signed/README.md) goes next to the agent, where the
# Firefox policy installs it from, unless the build already made one.
if [ ! -f "$DIST/raisesteady-guard.xpi" ] && [ -f "$ROOT/signed/raisesteady-guard-firefox.xpi" ]; then
  mkdir -p "$DIST" && cp "$ROOT/signed/raisesteady-guard-firefox.xpi" "$DIST/raisesteady-guard.xpi"
fi
echo "Firefox add-on in the installer: $([ -f "$DIST/raisesteady-guard.xpi" ] && echo yes || echo none)"

# The installer ships the licence notices (THIRD-PARTY-NOTICES.txt).
NOTICES="${KG_NOTICES:-$ROOT/THIRD-PARTY-NOTICES.txt}"
if [ ! -f "$NOTICES" ]; then
  echo "error: $NOTICES is missing. The installer must ship the third-party licence notices."
  exit 1
fi
GO_NOTICE="$ROOT/agent/NOTICE-go.txt"

# The welcome page says whether the extension is in the stores yet.
STORES=()
if grep -qE '"(chromeStoreId|edgeStoreId)": *"[a-p]{32}"' "$ROOT/shared/ext-ids.json"; then STORES=("-DEXT_IN_STORES"); fi

cd "$ROOT/installer"
"$MAKENSIS" -V2 -INPUTCHARSET UTF8 -WX ${STORES[@]+"${STORES[@]}"} \
  "-DVERSION=$VERSION" "-DDIST=$(winpath "$DIST")" "-DOUTFILE=$(winpath "$DIST/RaiseSteadyGuardSetup.exe")" \
  "-DNOTICES=$(winpath "$NOTICES")" "-DGO_NOTICE=$(winpath "$GO_NOTICE")" \
  raisesteady-guard.nsi
ls -l "$DIST/RaiseSteadyGuardSetup.exe"
