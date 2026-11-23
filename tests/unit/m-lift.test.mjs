// P2-271: a block page for a reason that ends by itself (time, video, bedtime, pause, homework)
// lets the child go on once that reason no longer holds; other block pages never lift this way.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { LIFTING, lifted, reasonFor } from '../../ext/background/guard.js';

const calm = { timeOver: false, videoOver: false, bedtime: false, paused: false, homework: false, allow: [], block: [] };

test('each lifting reason lifts only when it no longer holds', () => {
  const holds = { time: 'timeOver', video: 'videoOver', bedtime: 'bedtime', paused: 'paused', homework: 'homework' };
  assert.deepEqual([...LIFTING].sort(), Object.keys(holds).sort());
  for (const [reason, key] of Object.entries(holds)) {
    assert.equal(lifted(reason, { ...calm, [key]: true }), false, `${reason} still holds`);
    assert.equal(lifted(reason, calm), true, `${reason} lifted`);
  }
});

test('another reason still holding does not keep a lifted page shut (the network rules decide on arrival)', () => {
  // Time limit lifted, bedtime still on: the time page may go on; the bedtime rule then shows the bedtime page.
  assert.equal(lifted('time', { ...calm, bedtime: true }), true);
  assert.equal(reasonFor('example.org', { ...calm, bedtime: true }, 'https://example.org/'), 'bedtime');
});

test('content, adult, parent and topic pages never lift by themselves', () => {
  for (const reason of ['adult', 'gambling', 'bypass', 'social', 'content', 'parent', 'topic', 'shorts', 'curated', '', undefined]) {
    assert.equal(lifted(reason, calm), false, String(reason));
  }
  assert.equal(lifted('time', null), false);
});

test('the block page and the background list the same lifting reasons', () => {
  const page = readFileSync(new URL('../../ext/pages/blocked.js', import.meta.url), 'utf8');
  const m = /const LIFTING = (\[[^\]]*\]);/.exec(page);
  assert.ok(m, 'LIFTING in blocked.js');
  assert.deepEqual(JSON.parse(m[1].replace(/'/g, '"')), LIFTING);
});

test('the popup never sizes itself from the viewport (Firefox thin popup)', () => {
  const css = readFileSync(new URL('../../ext/pages/pages.css', import.meta.url), 'utf8');
  const html = readFileSync(new URL('../../ext/pages/popup.html', import.meta.url), 'utf8');
  assert.match(html, /<html[^>]*class="pp-root"/);
  assert.match(css, /html\.pp-root, html\.pp-root body \{ width: 22rem;/);
  // Below 12rem (Firefox still measuring) the fixed width stays; a narrow screen fits the screen. The range
  // must end a clear step below 22rem: a popup sized to its own 22rem (351.8px at 110% zoom) must not match.
  const mq = /@media \(min-width: 12rem\) and \(max-width: ([\d.]+)rem\) \{ html\.pp-root, html\.pp-root body \{ width: auto; \} \}/.exec(css);
  assert.ok(mq, 'the narrow-screen media query is there');
  assert.ok(Number(mq[1]) <= 21, `max-width ${mq[1]}rem ends at most 21rem`);
  // Both rems (media query and width) must mean the same size: no font-size on html or :root.
  for (const f of ['../../ext/pages/pages.css', '../../ext/ui/kit.css', '../../ext/ui/tokens.css']) {
    const src = readFileSync(new URL(f, import.meta.url), 'utf8');
    for (const block of src.matchAll(/(?:^|\n)\s*(?:html|:root)[^{,]*\{([^}]*)\}/g)) assert.ok(!/font-size/.test(block[1]), `${f}: no font-size on html or :root`);
  }
  const pp = /\n\.pp \{([^}]*)\}/.exec(css);
  assert.ok(pp && !/v[wh]\b/.test(pp[1]), '.pp has no vw/vh');
});
