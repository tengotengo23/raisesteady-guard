#!/usr/bin/env bash
# One-way export of this folder to the public open-source repository.
# kidguard/ inside the Nexodyne repository stays the home of the code; this
# copies its tracked files to a separate working tree, writes the public
# Windows, Linux and Android test workflows from ../.github/workflows/kidguard-{windows,linux,android}.yml (paths
# without the kidguard/ prefix), runs the secret scan and commits there.
#
#   tools/publish-oss.sh <target dir> "<commit message>" [--push]
#
# The target must be a git working tree (git init, remote set once by hand).
# Nothing is pushed without --push. Files deleted here are deleted there.
set -euo pipefail

KG="$(cd "$(dirname "$0")/.." && pwd)"
MONO="$(cd "$KG/.." && pwd)"
TARGET="${1:?target dir}"
MSG="${2:?commit message}"
PUSH="${3:-}"
SCAN="$HOME/Developer/claude-notes/bin/secret-scan.py"

[ -d "$TARGET/.git" ] || { echo "$TARGET is not a git working tree (git init it first)"; exit 1; }
case "$(cd "$TARGET" && pwd)" in "$MONO"|"$MONO"/*) echo "target must be outside the Nexodyne repository"; exit 1;; esac

[ -z "$(git -C "$TARGET" status --porcelain)" ] || { echo "$TARGET has uncommitted changes: commit or drop them first"; exit 1; }

LIST="$(mktemp)"
trap 'rm -f "$LIST"' EXIT
git -C "$KG" ls-files -z . > "$LIST"

# Rebuild the target from the tracked files: everything there is committed (checked
# above), so clearing it loses nothing; git then records what changed or was deleted.
find "$TARGET" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
rsync -a --from0 --files-from="$LIST" "$KG/" "$TARGET/"

# Public Windows, Linux and Android test workflows: same steps, root paths, run on push and pull requests.
mkdir -p "$TARGET/.github/workflows"
for os in windows linux android; do
python3 - "$MONO/.github/workflows/kidguard-$os.yml" "$TARGET/.github/workflows/$os.yml" "$os" <<'PY'
import re, sys
src, dst, name = sys.argv[1], sys.argv[2], sys.argv[3]
s = open(src, encoding='utf-8').read()
s = re.sub(r'(?ms)^on:\n.*?^(?=permissions:)', 'on:\n  workflow_dispatch:\n  push:\n    branches: [main]\n  pull_request:\n\n', s, count=1)
s = s.replace('kidguard/', '').replace('kidguard\\', '')
s = s.replace('name: kidguard-' + name, 'name: ' + name)
open(dst, 'w', encoding='utf-8').write(s)
PY
done

if [ -f "$SCAN" ]; then
  ( cd "$TARGET" && git add -A && python3 "$SCAN" --staged )
else
  echo "secret scan not found at $SCAN: stop"; exit 1
fi

cd "$TARGET"
git add -A
if git diff --cached --quiet; then
  echo "nothing changed"
else
  git commit -q -m "$MSG"
  git log --oneline -1
fi
if [ "$PUSH" = "--push" ]; then
  git push origin HEAD
fi
