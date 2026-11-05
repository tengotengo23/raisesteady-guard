#!/usr/bin/env bash
# One-way export of this folder to the public open-source repository.
# kidguard/ inside the Nexodyne repository stays the home of the code; this
# copies its committed files (kidguard/ at HEAD, read from git, never the working
# files: uncommitted changes are not published) to a separate working tree, writes
# the public Windows, Linux and Android test workflows from
# .github/workflows/kidguard-{windows,linux,android}.yml at HEAD (paths without the
# kidguard/ prefix), runs the secret scan and commits there.
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

# What is published: kidguard/ as committed at HEAD (its path inside the repository, for git).
REL="$(git -C "$KG" rev-parse --show-prefix)"; REL="${REL%/}"
[ -n "$REL" ] || { echo "cannot find kidguard/ inside its git repository"; exit 1; }
HEAD_ID="$(git -C "$MONO" rev-parse --short HEAD)"
if [ -n "$(git -C "$MONO" status --porcelain -- "$REL" .github/workflows)" ]; then
  echo "note: uncommitted changes in $REL/ or .github/workflows are not published (HEAD $HEAD_ID is)"
fi

# Rebuild the target from the committed files: everything there is committed (checked
# above), so clearing it loses nothing; git then records what changed or was deleted.
# git archive keeps file modes and symbolic links; COPYFILE_DISABLE keeps macOS metadata out.
find "$TARGET" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
git -C "$MONO" archive --format=tar HEAD -- "$REL" | COPYFILE_DISABLE=1 tar -x -C "$TARGET" --strip-components="$(awk -F/ '{print NF}' <<< "$REL")"

# Public Windows, Linux and Android test workflows: same steps, root paths, run on push and pull requests.
mkdir -p "$TARGET/.github/workflows"
WF="$(mktemp -d)"
trap 'rm -rf "$WF"' EXIT
for os in windows linux android; do
git -C "$MONO" show "HEAD:.github/workflows/kidguard-$os.yml" > "$WF/$os.yml"
python3 - "$WF/$os.yml" "$TARGET/.github/workflows/$os.yml" "$os" <<'PY'
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
