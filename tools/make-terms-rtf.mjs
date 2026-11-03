// Writes installer/terms-<lang>.rtf (the installer's terms page) from installer/terms-<lang>.txt.
// RTF with \u escapes, so Georgian and Russian show in every Windows RichEdit control.
// Lines starting with "# " are headings (bold). Run after editing a terms-*.txt file:
//   node tools/make-terms-rtf.mjs
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const LANGS = ['en', 'ka', 'ru'];
const INST = join(dirname(fileURLToPath(import.meta.url)), '..', 'installer');

function esc(text) {
  let out = '';
  for (const ch of text) {
    const c = ch.codePointAt(0);
    if (ch === '\\' || ch === '{' || ch === '}') out += '\\' + ch;
    else if (c < 128) out += ch;
    else if (c < 0x10000) out += `\\u${c > 32767 ? c - 65536 : c}?`;
    else { const s = String.fromCodePoint(c); out += `\\u${s.charCodeAt(0) - 65536}?\\u${s.charCodeAt(1) - 65536}?`; }
  }
  return out;
}

export function toRtf(text) {
  const body = text.replace(/\r\n/g, '\n').replace(/\n+$/, '').split('\n')
    .map((line) => (line.startsWith('# ') ? `{\\b ${esc(line.slice(2))}}` : esc(line)) + '\\par')
    .join('\n');
  return `{\\rtf1\\ansi\\ansicpg1252\\deff0{\\fonttbl{\\f0\\fnil\\fcharset0 Segoe UI;}}\\f0\\fs18\n${body}\n}\n`;
}

export const paths = (lang) => ({ txt: join(INST, `terms-${lang}.txt`), rtf: join(INST, `terms-${lang}.rtf`) });

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  for (const l of LANGS) {
    const p = paths(l);
    writeFileSync(p.rtf, toRtf(readFileSync(p.txt, 'utf8')));
    console.log('wrote', p.rtf);
  }
}
