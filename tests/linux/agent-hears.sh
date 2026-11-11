#!/bin/bash
# Asks the running service whether it heard from a browser's extension: signs
# in to the parent page with the PIN and reads the browser's row there, as a
# parent would. (The `status` command is a program of its own and cannot know
# this: the service keeps the reports' times in memory.) Used by the Linux test
# run (P2-194).
#
#   agent-hears.sh <browser name on the parent page> <PIN> [seconds to wait]
#
# Exit 0 when the row says the extension reported; 1 when it did not in time.
set -u
name="$1"; pin="$2"; wait="${3:-120}"
url="${KG_URL:-http://127.0.0.1:47615}"
jar="$(mktemp)"; page="$(mktemp)"
trap 'rm -f "$jar" "$page"' EXIT

code="$(curl -s -o /dev/null -w '%{http_code}' -c "$jar" -H "Origin: $url" -d lang=en --data-urlencode "pin=$pin" "$url/parent/login")"
curl -s -b "$jar" "$url/parent?lang=en" > "$page"
if ! grep -q 'action="/parent/logout"' "$page"; then
  echo "could not sign in to the parent page (the login answered $code)"; exit 1
fi
tries=$((wait / 3 + 1))
for _ in $(seq 1 "$tries"); do
  if grep -qE "<span>$name( <span class=\"muted\">[^<]*</span>)?</span> <span class=\"state active\">" "$page"; then
    echo "the agent hears from the extension in $name (the parent page says so)"; exit 0
  fi
  sleep 3
  curl -s -b "$jar" "$url/parent?lang=en" > "$page"
done
echo "the parent page's browser rows:"
grep -oE '<li><span>[^<]*(<span class="muted">[^<]*</span>)?</span> <span class="state [a-z]*">[^<]*</span>' "$page" | sed -E 's/<[^>]+>/ /g' | head -12
echo "the agent did not hear from the extension in $name within $wait s"
exit 1
