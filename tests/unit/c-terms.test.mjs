// The installer's terms page: each language has the no-warranty, privacy and licence parts,
// and the committed RTF files match their text sources (run tools/make-terms-rtf.mjs after an edit).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { LANGS, toRtf, paths } from '../../tools/make-terms-rtf.mjs';

test('terms RTF files are up to date with their text', () => {
  for (const l of LANGS) {
    const p = paths(l);
    assert.equal(readFileSync(p.rtf, 'utf8'), toRtf(readFileSync(p.txt, 'utf8')), `run node tools/make-terms-rtf.mjs (${l})`);
  }
});

test('every language says: made with AI, as is, no warranty, no responsibility, within the law, voluntary support, GPL-3.0', () => {
  const need = {
    en: ['made with AI', 'Support is voluntary', 'may not get updates', '"as is"', 'without any guarantee or warranty', 'take no responsibility', 'to the extent permitted by law', 'GPL-3.0'],
    ka: ['ხელოვნური ინტელექტით', 'მხარდაჭერა ნებაყოფლობითია', 'არ განახლდეს', '„როგორც არის“', 'გარანტიის გარეშე', 'არ იღებენ პასუხისმგებლობას', 'კანონით დაშვებულ ფარგლებში', 'GPL-3.0'],
    ru: ['создана с помощью ИИ', 'Поддержка добровольная', 'обновления могут не выходить', '«как есть»', 'без каких-либо гарантий', 'не несут ответственности', 'в пределах, допустимых законом', 'GPL-3.0'],
  };
  for (const l of LANGS) {
    const t = readFileSync(paths(l).txt, 'utf8');
    for (const w of need[l]) assert.ok(t.includes(w), `${l}: missing ${w}`);
    assert.equal((t.match(/^# /gm) || []).length, 3, `${l}: three headings`);
  }
});

test('RTF escapes non-ASCII as \\u and braces', () => {
  assert.match(toRtf('# Ä {x}\n'), /\{\\b \\u196\?\s\\\{x\\\}\}\\par/);
  assert.ok(toRtf('ა').includes('\\u4304?'));
});
