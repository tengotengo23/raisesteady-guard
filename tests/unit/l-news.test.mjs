// P2-252: news from RaiseSteady in the extension (standalone options page). Pure helpers only.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  FEED_URL, TTL, MAX_BYTES, MAX_TITLE, MAX_BODY, MAX_HIDDEN,
  parseFeed, safeLink, visibleItems, pickText, isStale, localToday, hideId, fetchFeed,
} from '../../ext/ui/news.js';

const feed = (...items) => JSON.stringify({ v: 1, items });
const good = { id: 'a-1', date: '2026-11-18', title: { en: 'T', ka: 'ტ' }, body: { en: 'one\ntwo' }, link: 'https://raisesteady.com/guard/' };
const it = (o) => ({ id: 'a', date: '2026-11-18', title: { en: 'T' }, body: { en: 'b' }, ...o });

test('a good feed parses; unknown fields are ignored', () => {
  const items = parseFeed(feed(good, { ...it({ id: 'b', until: '2026-12-31' }), extra: 5 }));
  assert.equal(items.length, 2);
  assert.equal(items[0].link, good.link);
  assert.equal(pickText(items[0].title, 'ka'), 'ტ');
  assert.equal(pickText(items[0].title, 'ru'), 'T');
  assert.equal(items[1].until, '2026-12-31');
  assert.equal(items[1].extra, undefined);
});

test('the whole file is ignored when it is not v 1, not JSON or too big', () => {
  for (const s of [
    JSON.stringify({ v: 2, items: [good] }), JSON.stringify({ v: '1', items: [] }), JSON.stringify({ items: [good] }),
    '<html>', 'null', feed(it({ body: { en: 'x'.repeat(MAX_BYTES) } })),
  ]) assert.equal(parseFeed(s), null, s.slice(0, 40));
  assert.deepEqual(parseFeed(feed()), []);
});

test('bad items are dropped; the limits themselves pass', () => {
  const bad = [
    it({ id: 'Bad' }), it({ id: 'a'.repeat(41) }), it({ date: '18.11.2026' }), it({ date: '2026-02-30' }), it({ until: 'soon' }),
    it({ title: { ka: 'T' } }), it({ title: { en: 'т'.repeat(MAX_TITLE + 1) } }), it({ body: { en: 'b'.repeat(MAX_BODY + 1) } }),
    it({ title: { en: 'line\nbreak' } }), it({ body: { en: 'bell\u0007' } }), it({ body: { en: 'a b' } }), it({ body: { en: '   ' } }),
    it({ title: 'T' }), 'just a string', null, [],
  ];
  for (const b of bad) assert.deepEqual(parseFeed(feed(b)), [], JSON.stringify(b));
  const ok = parseFeed(feed(it({ id: 'a'.repeat(40), title: { en: 'т'.repeat(MAX_TITLE), ru: 'я'.repeat(MAX_TITLE + 1) }, body: { en: 'b'.repeat(MAX_BODY) } })));
  assert.equal(ok.length, 1);
  assert.equal(pickText(ok[0].title, 'ru'), ok[0].title.en);
});

test('five items at most, in feed order', () => {
  const items = parseFeed(feed(...'abcdefg'.split('').map((id) => it({ id }))));
  assert.deepEqual(items.map((x) => x.id), ['a', 'b', 'c', 'd', 'e']);
});

test('only raisesteady.com and github.com/tengotengo23 links are kept', () => {
  for (const l of ['https://raisesteady.com/guard/', 'https://raisesteady.com/guard/?a=1#x', 'https://github.com/tengotengo23/raisesteady-guard/releases']) assert.ok(safeLink(l), l);
  for (const l of ['http://raisesteady.com/', 'https://raisesteady.com', 'https://raisesteady.com.evil.example/', 'https://raisesteady.com@evil.example/',
    'https://user@raisesteady.com/', 'https://raisesteady.com:8443/', 'https://evil.example/https://raisesteady.com/', 'javascript:alert(1)',
    'https://github.com/someoneelse/x', 'https://github.com/tengotengo23', 'https://github.com/tengotengo23/../evil/', 'https://github.com/tengotengo23/%2E%2E/evil/',
    'https://github.com/tengotengo23/./x', 'https://raisesteady.com/a b', 'https://raisesteady.com/"onmouseover=x', 'https://raisesteady.com/<x>',
    'https://raisesteady.com/\\evil', 'data:text/html,x', '', 'https://raisesteady.com/' + 'a'.repeat(300), 42, null]) assert.equal(safeLink(l), false, String(l));
  const items = parseFeed(feed(it({ link: 'https://evil.example/' })));
  assert.equal(items.length, 1);
  assert.equal(items[0].link, undefined);
});

test('hidden notes and notes on or after their until day are not shown', () => {
  const items = [{ id: 'past', until: '2026-11-17' }, { id: 'today', until: '2026-11-18' }, { id: 'later', until: '2026-11-19' }, { id: 'open' }, { id: 'hid' }];
  assert.deepEqual(visibleItems(items, '2026-11-18', ['hid']).map((x) => x.id), ['later', 'open']);
  assert.equal(localToday(new Date(2026, 0, 5, 23, 59)), '2026-01-05');
});

test('the cache is kept for 6 hours; hide lists stay small and clean', () => {
  const now = Date.UTC(2026, 10, 18, 10);
  assert.ok(isStale(undefined, now));
  assert.ok(isStale({ fetched: now - TTL }, now));
  assert.equal(isStale({ fetched: now - TTL + 60000 }, now), false);
  assert.ok(isStale({ fetched: now + 60000 }, now), 'a clock set back reads again');
  assert.deepEqual(hideId(['a', 'Bad'], 'b'), ['a', 'b']);
  assert.deepEqual(hideId(['a', 'b'], 'a'), ['b', 'a']);
  assert.equal(hideId([], '<x>'), null);
  const many = Array.from({ length: MAX_HIDDEN }, (_, i) => `n${i}`);
  const next = hideId(many, 'z');
  assert.equal(next.length, MAX_HIDDEN);
  assert.equal(next.at(-1), 'z');
  assert.equal(next[0], 'n1');
});

test('the feed is read once, with no cookies, no query and no redirects; problems give null', async () => {
  const calls = [];
  const res = (status, text, len) => ({ status, headers: { get: (h) => (h === 'content-length' ? len ?? null : null) }, text: async () => text });
  const ok = await fetchFeed(async (url, opts) => { calls.push([url, opts]); return res(200, feed(good)); });
  assert.equal(ok, feed(good));
  assert.equal(calls.length, 1);
  const [url, opts] = calls[0];
  assert.equal(url, FEED_URL);
  assert.equal(url.includes('?'), false);
  assert.equal(opts.credentials, 'omit');
  assert.equal(opts.redirect, 'error');
  assert.equal(opts.referrerPolicy, 'no-referrer');
  assert.ok(opts.signal);
  assert.equal(await fetchFeed(async () => res(500, 'oops')), null);
  assert.equal(await fetchFeed(async () => res(200, JSON.stringify({ v: 2, items: [] }))), null);
  assert.equal(await fetchFeed(async () => res(200, feed(good), String(MAX_BYTES + 1))), null);
  assert.equal(await fetchFeed(async () => { throw new TypeError('network'); }), null);
  // The review of 18 Nov: a reply without Content-Length is read in chunks and dropped past MAX_BYTES
  const streamed = (text) => ({ status: 200, headers: { get: () => null }, body: new Response(text).body, text: async () => { throw new Error('must not read whole'); } });
  assert.equal(await fetchFeed(async () => streamed(feed(good))), feed(good));
  assert.equal(await fetchFeed(async () => streamed(feed(good) + ' '.repeat(MAX_BYTES))), null);
});

test('the options page waits 10 minutes after a failed read, as the agent does', () => {
  const js = readFileSync(new URL('../../ext/pages/options.js', import.meta.url), 'utf8');
  assert.match(js, /newsTried/);
  assert.match(js, /nowMs - tried < RETRY\) return;\s*await newsSet\(\{ newsTried: nowMs \}\);\s*const text = await fetchFeed\(\)/);
});

test('every note the master panel can write passes this reader (tracker/news/routes.mjs)', async () => {
  const { checkPost } = await import('../../../tracker/news/routes.mjs');
  const now = Date.parse('2026-11-18T22:00:00Z');
  const notes = [
    { title: { en: 'Version 0.4.4', ka: 'ვერსია 0.4.4', ru: 'Версия 0.4.4' }, body: { en: 'One.\nTwo.', ka: 'ერთი', ru: 'Один' }, link: 'https://raisesteady.com/guard/', until: '2026-12-31' },
    { title: { en: 'x'.repeat(80) }, body: { en: 'y'.repeat(600) }, link: 'https://github.com/tengotengo23/raisesteady-guard/releases' },
  ];
  const items = notes.map((n) => { const r = checkPost(n, now); assert.equal(r.error, undefined, r.error); return r.item; });
  const kept = parseFeed(JSON.stringify({ v: 1, items }));
  assert.equal(kept.length, items.length);
  assert.ok(kept.every((it) => it.link), 'the links survive');
});

test('the options page shows news text with textContent only and links with noopener noreferrer', () => {
  const js = readFileSync(new URL('../../ext/pages/options.js', import.meta.url), 'utf8');
  const html = readFileSync(new URL('../../ext/pages/options.html', import.meta.url), 'utf8');
  assert.equal(/innerHTML|insertAdjacentHTML|outerHTML|document\.write/.test(js), false);
  assert.match(js, /rel: 'noopener noreferrer'/);
  assert.match(html, /<input id="newsSetup" type="checkbox" checked>/);
  // The address of the master panel never appears in the extension.
  assert.equal(/masterpanel/i.test(js + readFileSync(new URL('../../ext/ui/news.js', import.meta.url), 'utf8')), false);
  for (const l of ['en', 'ka', 'ru']) {
    const t = JSON.parse(readFileSync(new URL(`../../ext/i18n/core.${l}.json`, import.meta.url), 'utf8'));
    for (const k of ['newsTitle', 'newsAsk', 'newsHow', 'newsOn', 'newsOff', 'newsHide', 'newsMore', 'newsSetup']) assert.ok(t[k], `${l} ${k}`);
  }
});
