#!/bin/bash
# Evidence that a Chromium-family browser installed RaiseSteady Guard from the
# store by policy, gave it its managed settings, and keeps it switched on.
# Used by the Linux test run (.github/workflows/kidguard-linux.yml), P2-194.
#
#   ext-evidence.sh <name> <profile folder> <debugging port> <policy page> <extension id>
#
# The browser must already run with
#   --remote-debugging-port=<port> --user-data-dir=<profile folder>
#   --log-net-log=<profile folder>/netlog.json
#
# Three things are shown, each from the browser itself:
#   1. installed: the store's package is unpacked in the profile;
#   2. managed settings: the policy page lists the extension's own values
#      (when that page stays blank, as Edge's can, proof 3 shows them instead);
#   3. switched on and connected: the browser's network log has the agent's
#      answer 200 to the extension's report on port 47615 (the agent gives
#      that answer only for the right token from our extension's address; an
#      extension that is switched off, or one without managed settings, never
#      asks).
# Exit 1 when one of them is missing. The agent's token is never printed.
set -u
name="$1"; prof="$2"; port="$3"; page="$4"; id="$5"
here="$(cd "$(dirname "$0")" && pwd)"
text="/tmp/kg-policy-$port.txt"

mask() { sed -E 's/[0-9a-f]{64}/<token>/g'; }
fail() { echo "$name: $1"; exit 1; }

# 1. Installed (a store download: up to three minutes).
ver=""
for _ in $(seq 1 90); do
  m="$(find "$prof" -path "*/Extensions/$id/*/manifest.json" 2>/dev/null | head -1)"
  if [ -n "$m" ]; then ver="$(jq -r .version "$m")"; break; fi
  sleep 2
done
if [ -z "$ver" ]; then
  find "$prof" -maxdepth 3 -name Extensions -exec ls -la {} \; 2>/dev/null | head -20
  fail "the extension $id was not installed from the store within 3 minutes"
fi
echo "$name installed the extension from the store by itself: version $ver"

# 2. Managed settings, as the browser's policy page shows them.
ok=""
for _ in 1 2 3 4 5 6; do
  if node "$here/read-policy-page.mjs" cdp "$port" "$page" > "$text" 2> "$text.err" && grep -q agentPort "$text"; then ok=1; break; fi
  sleep 5
done
policy_seen="$ok"
if [ -z "$ok" ]; then
  echo "the policy page's text (its end):"; mask < "$text" | tail -c 3000; echo
  echo "the reader's last error:"; mask < "$text.err" | tail -5
  echo "what the browser has open:"; node "$here/read-policy-page.mjs" pages "$port" 2>&1 | mask | head -20
  # Not a pass yet: proof 3 decides. The agent answers 200 only to the token, which the
  # extension gets from nowhere but its managed settings.
  echo "$name: the policy page does not show the managed settings (agentPort); proof 3 must show them"
else
  echo "$name gives the extension its managed settings"
fi

# 3. Switched on, and it reaches the agent.
ok=""
for _ in $(seq 1 75); do
  if node "$here/netlog-agent.mjs" "$prof/netlog.json" 47615 > "$text.net" 2>&1; then ok=1; break; fi
  sleep 2
done
if [ -z "$ok" ]; then
  ls -la "$prof/netlog.json" 2>/dev/null || echo "no network log at $prof/netlog.json"
  echo "what the browser asked the agent, and the answers:"; mask < "$text.net" | head -20
  fail "the agent did not answer 200 to the extension's report within 150 s (switched off, without its managed settings, or refused)"
fi
mask < "$text.net" | head -8
echo "$name: the extension is switched on, and the agent on port 47615 accepts its report"
if [ -z "$policy_seen" ]; then
  echo "$name gives the extension its managed settings: shown by the agent's 200, which needs the token from them (the policy page stayed blank)"
fi
