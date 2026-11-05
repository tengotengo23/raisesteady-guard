#!/usr/bin/env bash
# Converts the RS emoji the app shows (from the repo's emoji/svg/) to Android VectorDrawables in
# app/src/main/res/drawable/rs_*.xml (design 4.6). Run once after an emoji changes, then commit
# the output. Uses svg2vectordrawable 2.9.1 (MIT, https://github.com/Ashung/svg2vectordrawable),
# installed into a temporary folder with install scripts off. Needs Node and npm.
#
#   bash kidguard/android/tools/emoji-to-vd.sh
set -euo pipefail

AND="$(cd "$(dirname "$0")/.." && pwd)"
REPO="$(cd "$AND/../.." && pwd)"
SVG="$REPO/emoji/svg"
OUT="$AND/app/src/main/res/drawable"
VERSION=2.9.1

# name in the app, then the emoji's code point file
EMOJI="rs_hourglass:emoji_u23f3 rs_moon:emoji_u1f319 rs_books:emoji_u1f4da rs_alarm:emoji_u23f0 rs_lock:emoji_u1f512 rs_check:emoji_u2705"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
( cd "$TMP" && npm init -y >/dev/null && npm install --silent --ignore-scripts --no-audit --no-fund "svg2vectordrawable@$VERSION" )

# svg2vectordrawable 2.9.1 drops <circle> and <ellipse> and an ellipse's rotate(): they become
# arc paths first, the rotation applied to the points and the arcs' x-axis angle (same shape).
SHAPES='const fs=require("fs");const [src,dst]=process.argv.slice(1);const n=v=>+(+v).toFixed(3);
const at=(a,k)=>{const m=a.match(new RegExp("\\s"+k+"=\"([^\"]*)\""));return m?m[1]:null};
const strip=(a,ks)=>ks.reduce((s,k)=>s.replace(new RegExp("\\s"+k+"=\"[^\"]*\""),""),a);
let s=fs.readFileSync(src,"utf8");
s=s.replace(/<(circle|ellipse)(\s[^>]*?)\/>/g,(m,tag,a)=>{const cx=+(at(a,"cx")||0),cy=+(at(a,"cy")||0);
const rx=+(tag==="circle"?at(a,"r"):at(a,"rx")),ry=+(tag==="circle"?at(a,"r"):at(a,"ry"));
let ang=0,px=0,py=0;const t=at(a,"transform");
if(t){const r=t.match(/^rotate\(\s*(-?[\d.]+)(?:[\s,]+(-?[\d.]+)[\s,]+(-?[\d.]+))?\s*\)$/);if(!r)throw new Error("transform "+t);ang=+r[1];px=+(r[2]||0);py=+(r[3]||0)}
const k=ang*Math.PI/180,rot=(x,y)=>[n(px+(x-px)*Math.cos(k)-(y-py)*Math.sin(k)),n(py+(x-px)*Math.sin(k)+(y-py)*Math.cos(k))];
const [ax,ay]=rot(cx-rx,cy),[bx,by]=rot(cx+rx,cy);
const d=`M${ax} ${ay}A${n(rx)} ${n(ry)} ${n(ang)} 1 0 ${bx} ${by}A${n(rx)} ${n(ry)} ${n(ang)} 1 0 ${ax} ${ay}Z`;
return `<path d="${d}"${strip(a,["cx","cy","r","rx","ry","transform"])}/>`});fs.writeFileSync(dst,s);'

for pair in $EMOJI; do
  name="${pair%%:*}"; src="$SVG/${pair#*:}.svg"
  [ -f "$src" ] || { echo "missing $src"; exit 1; }
  node -e "$SHAPES" "$src" "$TMP/$name.svg"
  "$TMP/node_modules/.bin/s2v" -p 2 -i "$TMP/$name.svg" -o "$TMP/$name.xml" >/dev/null
  {
    echo '<?xml version="1.0" encoding="utf-8"?>'
    echo "<!-- RS emoji ${pair#*:} (emoji/svg), made by tools/emoji-to-vd.sh with svg2vectordrawable $VERSION. Do not edit by hand. -->"
    grep -v '^<?xml' "$TMP/$name.xml"
  } > "$OUT/$name.xml"
  echo "$OUT/$name.xml"
done
