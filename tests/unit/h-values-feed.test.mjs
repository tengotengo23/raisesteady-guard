// Values videos (P2-175): daily rotation, band and language, variety, the mix with channel videos, the pool file,
// and the pool builder's fixed filters.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import vm from 'node:vm';
import { valuesPicks, interleave, seeded, localDay } from '../../ext/background/videos.js';
import { dropReason, collect, buildPool } from '../../tools/update-values-videos.mjs';

const src = readFileSync(new URL('../../ext/content/topics.js', import.meta.url), 'utf8');
const topicsData = JSON.parse(readFileSync(new URL('../../shared/topics.json', import.meta.url), 'utf8'));
const sandbox = {}; sandbox.globalThis = sandbox; vm.runInNewContext(src, sandbox);
const T = sandbox.KGTopics.create(topicsData);

const id = (i) => `vid${String(i).padStart(8, '0')}`;
const POOL = [];
for (let i = 0; i < 60; i++) POOL.push({ id: id(i), title: `Fable ${i}`, channel: `Ch ${i % 12}`, channelId: `UC${String(i % 12).padStart(22, 'x')}`, lang: i < 40 ? 'en' : 'ru', bands: i % 2 ? ['3-5', '6-8'] : ['9-12'], themes: [['habits', 'work', 'courage'][i % 3]] });

test('same day, band and language: the same picks; another day: others', () => {
  const a = valuesPicks(POOL, { band: '3-5', lang: 'en', day: '2026-11-05', n: 8 });
  const b = valuesPicks(POOL, { band: '3-5', lang: 'en', day: '2026-11-05', n: 8 });
  const c = valuesPicks(POOL, { band: '3-5', lang: 'en', day: '2026-11-06', n: 8 });
  assert.equal(a.length, 8);
  assert.deepEqual(a.map((v) => v.id), b.map((v) => v.id));
  assert.notDeepEqual(a.map((v) => v.id), c.map((v) => v.id));
  for (const v of a) assert.equal(v.src, 'values');
});

test('over a month every video of the band is shown (rotation through the pool)', () => {
  const seen = new Set();
  for (let d = 1; d <= 30; d++) for (const v of valuesPicks(POOL, { band: '3-5', lang: 'en', day: `2026-11-${String(d).padStart(2, '0')}`, n: 8 })) seen.add(v.id);
  const band = POOL.filter((v) => v.bands.includes('3-5') && v.lang === 'en').length;
  assert.ok(seen.size >= band * 0.9, `${seen.size} of ${band}`);
});

test('only the band; own language first, English fills up; varied channels; exclude and topic titles left out', () => {
  const ru = valuesPicks(POOL, { band: '3-5', lang: 'ru', day: '2026-11-05', n: 14 });
  assert.ok(ru.every((v) => POOL.find((p) => p.id === v.id).bands.includes('3-5')));
  const langs = ru.map((v) => POOL.find((p) => p.id === v.id).lang);
  assert.equal(langs.filter((l) => l === 'ru').length, 10); // all 10 Russian 3-5 videos first
  assert.ok(langs.slice(0, 10).every((l) => l === 'ru'), langs.join(','));
  const few = valuesPicks(POOL, { band: '9-12', lang: 'en', day: '2026-11-05', n: 6 });
  assert.equal(new Set(few.map((v) => v.channelId)).size, 6, 'six different channels');
  const ex = valuesPicks(POOL, { band: '9-12', lang: 'en', day: '2026-11-05', n: 6, exclude: few.map((v) => v.id) });
  assert.ok(ex.every((v) => !few.some((f) => f.id === v.id)));
  const bad = [...POOL, { id: 'halloween01', title: 'Halloween Fable', channel: 'X', channelId: 'UCx', lang: 'en', bands: ['3-5'], themes: ['work'] }];
  for (let d = 1; d <= 30; d++) assert.ok(!valuesPicks(bad, { band: '3-5', lang: 'en', day: `2026-11-${d}`, n: 30, topics: T }).some((v) => v.id === 'halloween01'));
});

test('interleave: two channel videos, one values video; no doubles; limit', () => {
  const ch = Array.from({ length: 10 }, (_, i) => ({ id: `c${i}` })), va = Array.from({ length: 4 }, (_, i) => ({ id: `v${i}` }));
  assert.deepEqual(interleave(ch, va, 9).map((v) => v.id), ['c0', 'c1', 'v0', 'c2', 'c3', 'v1', 'c4', 'c5', 'v2']);
  assert.deepEqual(interleave([{ id: 'a' }], [{ id: 'a' }, { id: 'b' }], 5).map((v) => v.id), ['a', 'b']);
  assert.equal(interleave([], va, 3).length, 3);
});

test('seeded and localDay are stable', () => {
  const r1 = seeded('x'), r2 = seeded('x');
  assert.equal(r1(), r2());
  assert.match(localDay(Date.parse('2026-11-05T12:00:00Z')), /^2026-11-0[456]$/);
});

test('the pool builder filters: length, views, topics, junk, religious, dropped channels, caps', () => {
  const ok = { id: 'abcdefghijk', title: 'The Tortoise and the Hare | Aesop Fables', channel: 'Little Fox', channelId: `UC${'a'.repeat(22)}`, lengthSec: 600, views: 100000 };
  assert.equal(dropReason(ok, 'en', 'kids'), '');
  assert.equal(dropReason({ ...ok, lengthSec: 30 }, 'en', 'kids'), 'length');
  assert.equal(dropReason({ ...ok, views: 500 }, 'en', 'kids'), 'views');
  assert.equal(dropReason({ ...ok, views: 1500 }, 'ka', 'kids'), '');
  assert.equal(dropReason({ ...ok, views: 30000 }, 'en', 'teen'), 'views');
  assert.equal(dropReason({ ...ok, title: 'Halloween Fables' }, 'en', 'kids'), 'topic');
  assert.equal(dropReason({ ...ok, desc: 'a kids yoga story' }, 'en', 'kids'), 'topic');
  assert.equal(dropReason({ ...ok, title: 'Fable PRANK gone wrong' }, 'en', 'kids'), 'junk');
  assert.equal(dropReason({ ...ok, title: 'Bible stories for kids' }, 'en', 'kids'), 'religious');
  assert.equal(dropReason({ ...ok, title: 'Притча о блудном сыне. Православный мультфильм' }, 'ru', 'kids'), 'religious');
  assert.equal(dropReason({ ...ok, channel: 'Sesame Street' }, 'en', 'kids'), 'dropped channel');
  assert.equal(dropReason({ ...ok, title: 'THE TORTOISE AND THE HARE STORY' }, 'en', 'kids'), 'caps');
  const rows = Array.from({ length: 5 }, (_, i) => ({ v: { ...ok, id: `abcdefghij${i}` }, lang: 'en', age: 'kids', theme: 'habits', q: 'x' }));
  rows.push({ v: { ...ok, id: 'abcdefghij0' }, lang: 'en', age: 'teen', theme: 'work', q: 'y' });
  const { candidates, drops } = collect(rows, { kids: ['3-5', '6-8', '9-12'], teen: ['13-15', '16-17'] });
  assert.equal(candidates.length, 3, 'at most 3 per channel and theme');
  assert.equal(drops.cap, 2);
  const merged = candidates.find((c) => c.id === 'abcdefghij0');
  assert.deepEqual(merged.themes, ['habits', 'work']);
  assert.ok(merged.bands.includes('16-17'));
  assert.deepEqual(buildPool(candidates, { approve: ['abcdefghij0', 'abcdefghij1'], reject: ['abcdefghij1'] }).map((v) => v.id), ['abcdefghij0']);
});

test('the pool file: reviewed, valid, no topic titles, every band and the three languages covered', { skip: !existsSync(new URL('../../shared/values-videos.json', import.meta.url)) && 'no pool yet' }, () => {
  const pool = JSON.parse(readFileSync(new URL('../../shared/values-videos.json', import.meta.url), 'utf8'));
  const review = JSON.parse(readFileSync(new URL('../../shared/values-review.json', import.meta.url), 'utf8'));
  const approved = new Set(review.approve);
  assert.ok(pool.videos.length >= 150, `${pool.videos.length} videos`);
  const ids = new Set();
  for (const v of pool.videos) {
    assert.match(v.id, /^[\w-]{11}$/);
    assert.ok(!ids.has(v.id), `double ${v.id}`); ids.add(v.id);
    assert.ok(approved.has(v.id), `not reviewed: ${v.id}`);
    assert.equal(T.matchTitle(`${v.title} ${v.channel}`).hit, false, v.title);
    assert.ok(['en', 'ru', 'ka'].includes(v.lang));
    assert.ok(v.bands.length && v.themes.length);
  }
  for (const band of ['3-5', '6-8', '9-12']) for (const lang of ['en', 'ru']) {
    const n = pool.videos.filter((v) => v.lang === lang && v.bands.includes(band)).length;
    assert.ok(n >= 20, `${lang} ${band}: ${n}`);
  }
  assert.ok(pool.videos.some((v) => v.lang === 'ka'), 'Georgian');
  assert.ok(pool.videos.some((v) => v.bands.includes('13-15')), 'teens');
});
