// feed parser on a saved real feed, Shorts filter, channel mixing, channel choice.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseFeed, shortFromLink, dropShorts, mixVideos, channelsFor, decodeEntities, pool } from '../../ext/background/videos.js';

const FIX = new URL('../e2e/fixtures-b/', import.meta.url);
const xml = readFileSync(new URL('feed-sesame.xml', FIX), 'utf8');

test('parses a real YouTube channel feed (Sesame Street, saved 3 Nov 2026)', () => {
  const f = parseFeed(xml);
  assert.equal(f.channelId, 'UCoookXUzPciGrEZEXmh4Jjg');
  assert.equal(f.title, 'Sesame Street');
  assert.equal(f.videos.length, 15);
  const v = f.videos[0];
  assert.equal(v.id, '1FqWyIumLZs');
  assert.match(v.title, /^Elmo's Favorite Nursery Rhymes/);
  assert.equal(v.channel, 'Sesame Street');
  assert.equal(v.published, Date.parse('2026-11-01T12:30:08+00:00'));
  assert.equal(v.short, false);
  assert.equal(f.videos[1].short, true); // r65hiAj3k94 is linked as /shorts/
  for (const x of f.videos) { assert.match(x.id, /^[\w-]{11}$/); assert.ok(!('description' in x)); }
});

test('rejects things that are not feeds and survives hostile text', () => {
  assert.equal(parseFeed('<html><body>consent</body></html>'), null);
  assert.equal(parseFeed(null), null);
  const evil = '<feed><entry><yt:videoId>abcdefghijk</yt:videoId><title>&lt;img src=x onerror=alert(1)&gt; &amp;#38; ok</title>' +
    '<link rel="alternate" href="https://www.youtube.com/watch?v=abcdefghijk"/><author><name>A&amp;B</name></author><published>bad</published></entry>' +
    '<entry><yt:videoId>../../etc/pa</yt:videoId></entry></feed>';
  const f = parseFeed(evil);
  assert.equal(f.videos.length, 1);
  assert.equal(f.videos[0].title, '<img src=x onerror=alert(1)> &#38; ok'); // text only: the grid sets textContent
  assert.equal(f.videos[0].channel, 'A&B');
  assert.equal(f.videos[0].published, 0);
});

test('decodes numeric and named entities, drops invalid code points', () => {
  assert.equal(decodeEntities('&#x10D3;&#4304; &quot;x&quot; &apos;y&apos; &bogus; &#0; &#xD800;'), 'და "x" \'y\' &bogus;  ');
});

test('Shorts filter: link says it, cache answers the rest, unknown is dropped', () => {
  assert.equal(shortFromLink('https://www.youtube.com/shorts/r65hiAj3k94'), true);
  assert.equal(shortFromLink('https://www.youtube.com/watch?v=1FqWyIumLZs'), false);
  assert.equal(shortFromLink('https://example.com/watch?v=1FqWyIumLZs'), null);
  assert.equal(shortFromLink(''), null);
  const f = parseFeed(xml);
  const kept = dropShorts(f.videos);
  assert.equal(kept.length, f.videos.filter((v) => v.short === false).length);
  assert.ok(kept.length >= 5 && kept.length < 15, `kept ${kept.length}`);
  assert.ok(!kept.some((v) => v.id === 'r65hiAj3k94'));
  const vids = [{ id: 'aaaaaaaaaaa', short: null }, { id: 'bbbbbbbbbbb', short: null }, { id: 'ccccccccccc', short: null }];
  assert.deepEqual(dropShorts(vids, { aaaaaaaaaaa: false, bbbbbbbbbbb: true }).map((v) => v.id), ['aaaaaaaaaaa']);
});

test('mixing: one busy channel cannot fill the grid; newest first; own language first', () => {
  const mk = (ch, n, start, step = 3600e3) => Array.from({ length: n }, (_, i) => ({ id: `${ch}${String(i).padStart(10 - ch.length + 1, '0')}`.slice(0, 11).padEnd(11, 'x'), channelId: ch, published: start - i * step }));
  const now = Date.parse('2026-11-03T00:00:00Z');
  const busy = { channelId: 'A', lang: 'en', videos: mk('A', 15, now, 60e3) };
  const quiet1 = { channelId: 'B', lang: 'en', videos: mk('B', 3, now - 86400e3) };
  const quiet2 = { channelId: 'C', lang: 'ru', videos: mk('C', 3, now - 5 * 86400e3) };
  const out = mixVideos([busy, quiet1, quiet2], { limit: 6, lang: 'en' });
  assert.equal(out.length, 6);
  const count = (c) => out.filter((v) => v.channelId === c).length;
  assert.deepEqual([count('A'), count('B'), count('C')], [2, 2, 2]);
  assert.equal(out[0].channelId, 'A');
  const ru = mixVideos([busy, quiet1, quiet2], { limit: 3, lang: 'ru' });
  assert.equal(ru[0].channelId, 'C'); // the child's language leads each round
  assert.equal(new Set(out.map((v) => v.id)).size, out.length);
  const dup = mixVideos([{ channelId: 'A', videos: [busy.videos[0]] }, { channelId: 'B', videos: [busy.videos[0]] }], { limit: 5 });
  assert.equal(dup.length, 1);
  assert.equal(mixVideos([], { limit: 4 }).length, 0);
  const real = mixVideos([{ channelId: 'UCoookXUzPciGrEZEXmh4Jjg', lang: 'en', videos: dropShorts(parseFeed(xml).videos) }], { limit: 4 });
  assert.equal(real.length, 4);
  assert.ok(real[0].published >= real[1].published);
});

test('channels for a band: its language plus English, valid ids only', () => {
  const data = JSON.parse(readFileSync(new URL('../../shared/channels.json', import.meta.url), 'utf8')).channels;
  const en = channelsFor(data, '3-5', 'en');
  const ru = channelsFor(data, '3-5', 'ru');
  const ka = channelsFor(data, '3-5', 'ka');
  assert.ok(en.length > 0 && en.every((c) => c.bands.includes('3-5') && c.lang === 'en'));
  assert.ok(ru.length > en.length && ru.some((c) => c.lang === 'ru'));
  assert.equal(ka.length, en.length);
  assert.equal(channelsFor([{ id: 'bad', bands: ['3-5'], lang: 'en' }], '3-5', 'en').length, 0);
  for (const band of ['3-5', '6-8', '9-12', '13-15', '16-17']) assert.ok(channelsFor(data, band, 'en').length >= 3, band);
});

test('pool runs at most n jobs at once', async () => {
  let live = 0, peak = 0;
  const r = await pool([1, 2, 3, 4, 5, 6, 7], 3, 0, async (x) => { live++; peak = Math.max(peak, live); await new Promise((s) => setTimeout(s, 5)); live--; if (x === 4) throw new Error('x'); return x * 2; });
  assert.equal(peak, 3);
  assert.deepEqual(r, [2, 4, 6, undefined, 10, 12, 14]);
});
