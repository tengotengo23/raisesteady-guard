// P2-161: built-in videos (merge with the feeds, fallback, what the grid shows, offline text) and the
// topic filter in the background (config switch, rules, searches, helplines).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { mergeLists, gridState, builtinVideo, mixVideos, builtinAgeDays, BUILTIN_MAX_AGE_DAYS } from '../../ext/background/videos.js';
import { resolveConfig } from '../../ext/background/config.js';
import { buildRules, reasonFor, RULE_ID } from '../../ext/background/guard.js';
import { topicSearch, helplineHost, topicsValue } from '../../ext/background/topics.js';

const sandbox = {};
sandbox.globalThis = sandbox;
vm.runInNewContext(readFileSync(new URL('../../ext/content/topics.js', import.meta.url), 'utf8'), sandbox);
const topicData = JSON.parse(readFileSync(new URL('../../shared/topics.json', import.meta.url), 'utf8'));
const T = sandbox.KGTopics.create(topicData);
const bands = JSON.parse(readFileSync(new URL('../../shared/age-bands.json', import.meta.url), 'utf8')).bands;
const builtinFile = JSON.parse(readFileSync(new URL('../../shared/videos.json', import.meta.url), 'utf8'));
const channelsFile = JSON.parse(readFileSync(new URL('../../shared/channels.json', import.meta.url), 'utf8'));

const A = 'UCaaaaaaaaaaaaaaaaaaaaaa', B = 'UCbbbbbbbbbbbbbbbbbbbbbb';
const chans = [{ id: A, lang: 'en' }, { id: B, lang: 'en' }];
const vid = (id, title, day) => ({ id, title, channel: 'Ch', published: Date.parse(`2026-10-${day}T00:00:00Z`) });
const saved = (id, title, day) => ({ id, title, channel: 'Saved ch', published: `2026-10-${day}` });

test('built-in video: date string to time; bad entries dropped', () => {
  assert.deepEqual(builtinVideo(saved('aaaaaaaaaa1', 'Volcanoes', '20'), A), { id: 'aaaaaaaaaa1', title: 'Volcanoes', channel: 'Saved ch', channelId: A, published: Date.parse('2026-10-20T00:00:00Z') });
  assert.equal(builtinVideo({ id: 'short', title: 'x' }, A), null);
  assert.equal(builtinVideo({ id: 'aaaaaaaaaa1' }, A), null);
  assert.equal(builtinVideo({ id: 'aaaaaaaaaa1', title: 'x', published: 'soon' }, A).published, 0);
});

test('feeds down: the built-in list alone fills the grid, marked saved', () => {
  const lists = mergeLists(chans, { [A]: { videos: [], failAt: 1 } }, { [A]: [saved('aaaaaaaaaa1', 'Rivers', '20')], [B]: [saved('bbbbbbbbbb1', 'Stars', '21'), saved('bbbbbbbbbb2', 'Moon', '19')] });
  const out = mixVideos(lists, { limit: 12 });
  assert.equal(out.length, 3);
  assert.ok(out.every((v) => v.src === 'saved'));
  assert.equal(gridState({ videos: out, saved: true }, true), 'grid-saved');
});

test('feeds up: merged with the built-in list, no doubles, the feed copy wins', () => {
  const feeds = { [A]: { videos: [vid('aaaaaaaaaa1', 'Rivers (new title)', '22'), vid('aaaaaaaaaa2', 'Lakes', '23')] } };
  const lists = mergeLists(chans, feeds, { [A]: [saved('aaaaaaaaaa1', 'Rivers', '20'), saved('aaaaaaaaaa3', 'Seas', '10')] });
  const a = lists[0].videos;
  assert.deepEqual(a.map((v) => v.id), ['aaaaaaaaaa1', 'aaaaaaaaaa2', 'aaaaaaaaaa3']);
  assert.equal(a[0].title, 'Rivers (new title)');
  assert.equal(a[0].src, 'feed');
  assert.equal(a[2].src, 'saved');
  const out = mixVideos(lists, { limit: 12 });
  assert.equal(out.every((v) => v.src === 'saved'), false);
});

test('titles on the topic list are left out of the grid (feed and built-in)', () => {
  const feeds = { [A]: { videos: [vid('aaaaaaaaaa1', 'Coming out as transgender', '22'), vid('aaaaaaaaaa2', 'Lakes', '23')] } };
  const lists = mergeLists(chans, feeds, { [B]: [saved('bbbbbbbbbb1', 'Pride Month special', '20'), saved('bbbbbbbbbb2', 'Pride and Prejudice, read aloud', '20')] }, T);
  assert.deepEqual(lists.flatMap((l) => l.videos.map((v) => v.id)), ['aaaaaaaaaa2', 'bbbbbbbbbb2']);
  const off = mergeLists(chans, feeds, {}, null);
  assert.equal(off[0].videos.length, 2, 'filter off: all titles');
});

test('the offline text only when the browser is offline and nothing can be shown', () => {
  assert.equal(gridState({ videos: [] }, false), 'offline');
  assert.equal(gridState({ videos: [] }, true), 'empty');
  assert.equal(gridState(null, true), 'empty');
  assert.equal(gridState({ videos: [{ id: 'x' }], saved: false }, false), 'grid', 'cached videos show even offline');
});

test('the built-in file: built by the tool, only curated channels, valid ids, no topic titles', () => {
  const ids = new Set(channelsFile.channels.map((c) => c.id));
  const entries = Object.entries(builtinFile.channels);
  assert.ok(entries.length >= 40, `${entries.length} channels`);
  for (const [ch, list] of entries) {
    assert.ok(ids.has(ch), ch);
    assert.ok(list.length >= 1 && list.length <= 12, `${ch}: ${list.length}`);
    for (const v of list) {
      assert.match(v.id, /^[\w-]{11}$/);
      assert.ok(v.title && v.title.length <= 200);
      assert.match(v.published, /^(\d{4}-\d{2}-\d{2})?$/);
      assert.equal(T.matchTitle(v.title).hit, false, v.title);
    }
  }
});

test('config: the topic filter is on by default for every band, managed and standalone; the switch turns it off', () => {
  for (const b of bands) {
    assert.equal(resolveConfig({ bands, local: { ageBand: b.id } }).topics, true, b.id);
    assert.equal(resolveConfig({ bands, managed: { agentToken: 'a'.repeat(64) }, agent: { band: b.id } }).topics, true, `${b.id} old agent`);
  }
  assert.equal(resolveConfig({ bands, managed: { agentToken: 'a'.repeat(64) }, agent: { band: '9-12', topics: false } }).topics, false);
  assert.equal(resolveConfig({ bands, local: { ageBand: '13-15', topics: false } }).topics, false);
});

test('rules: helplines always open (above pause and bedtime); topic sites go to the topic page unless allowed', () => {
  const s = { paused: true, bedtime: true, helplines: T.helplines(), topicDomains: T.domains(), allow: ['hrc.org'] };
  const rules = buildRules(s);
  const help = rules.find((r) => r.id === RULE_ID.helpline);
  assert.ok(help && help.action.type === 'allow' && help.priority > rules.find((r) => r.id === RULE_ID.paused).priority);
  assert.ok(help.condition.requestDomains.includes('findahelpline.com'));
  const topic = rules.find((r) => r.id === RULE_ID.topic);
  assert.match(topic.action.redirect.extensionPath, /reason=topic$/);
  assert.ok(topic.condition.requestDomains.includes('glaad.org'));
  assert.ok(!topic.condition.requestDomains.includes('hrc.org'), 'the parent allowed it');
  assert.equal(reasonFor('www.findahelpline.com', s), null);
  assert.equal(reasonFor('glaad.org', { ...s, paused: false, bedtime: false }), 'topic');
  assert.equal(reasonFor('glaad.org', { ...s, paused: false, bedtime: false, topicDomains: [] }), null, 'filter off');
  assert.equal(buildRules({ helplines: [], topicDomains: [] }).length, 0);
});

test('searches on the topic (every engine) are found; ordinary ones, allowed sites and the off switch pass', () => {
  const cfg = { topics: true, allow: [] };
  for (const u of ['https://www.google.com/search?q=lgbt', 'https://www.bing.com/search?q=am+i+gay', 'https://duckduckgo.com/?q=transgender', 'https://search.yahoo.com/search?p=gender+identity',
    'https://www.ecosia.org/search?q=coming+out+as+bi', 'https://yandex.ru/search/?text=%D0%BB%D0%B3%D0%B1%D1%82', 'https://www.youtube.com/results?search_query=pride+month', 'https://www.google.ge/search?q=%E1%83%9A%E1%83%92%E1%83%91%E1%83%A2']) {
    assert.equal(topicSearch(u, cfg, T, new URL(u).hostname.replace(/^www\./, '')), true, u);
  }
  for (const u of ['https://www.google.com/search?q=volcano', 'https://www.google.com/search?q=gay-lussac+law', 'https://www.google.com/maps?q=lgbt', 'https://example.org/?q=lgbt']) {
    assert.equal(topicSearch(u, cfg, T, new URL(u).hostname), false, u);
  }
  assert.equal(topicSearch('https://www.google.com/search?q=lgbt', { topics: false, allow: [] }, T, 'google.com'), false);
  assert.equal(topicSearch('https://www.google.com/search?q=lgbt', { topics: true, allow: ['google.com'] }, T, 'google.com'), false);
});

test('helplines are exempt in every path: the guard used by the content check, the topic check, video time and searches', () => {
  for (const h of ['findahelpline.com', 'www.findahelpline.com', 'chat.findahelpline.com', 'samaritans.org', '116111.pl', 'kek-vonal.hu', 'thetrevorproject.org']) {
    assert.equal(helplineHost(h.replace(/^www\./, ''), T), true, h);
  }
  for (const h of ['notfindahelpline.com', 'google.com', 'glaad.org', '']) assert.equal(helplineHost(h, T), false, h);
  assert.equal(helplineHost('findahelpline.com', null), false);
  // a search typed on a helpline's own search page is never sent to the topic page
  assert.equal(topicSearch('https://findahelpline.com/search?q=lgbt', { topics: true, allow: [] }, T, 'findahelpline.com'), false);
  // sweeps of open tabs (reasonFor) leave helplines alone in every state
  for (const st of [{ paused: true }, { bedtime: true }, { homework: true }, { timeOver: true }, { block: ['findahelpline.com'] }, { content: ['findahelpline.com'] }, { topicDomains: ['findahelpline.com'] }]) {
    assert.equal(reasonFor('findahelpline.com', { helplines: T.helplines(), ...st }, 'https://findahelpline.com/'), null, JSON.stringify(st));
  }
});

test('the page checks always get the helpline list, also with the topic filter off', () => {
  const on = topicsValue({ topics: true, allow: ['x.org'] }, topicData);
  assert.equal(on.on, true); assert.ok(on.data); assert.deepEqual(on.allow, ['x.org']);
  const off = topicsValue({ topics: false, allow: [] }, topicData);
  assert.equal(off.on, false); assert.equal(off.data, null);
  assert.ok(off.helplines.includes('findahelpline.com'));
  assert.equal(topicsValue(null, null).helplines.length, 0);
});

test('freshness guard: days since the built-in list was made; store builds refuse it after 120 days', () => {
  const now = Date.parse('2026-11-04T12:00:00Z');
  assert.equal(builtinAgeDays('2026-11-04', now), 0);
  assert.equal(builtinAgeDays('2026-07-07', now), 120);
  assert.equal(builtinAgeDays('', now), Infinity);
  assert.equal(builtinAgeDays('soon', now), Infinity);
  assert.equal(BUILTIN_MAX_AGE_DAYS, 120);
  assert.match(builtinFile.built, /^\d{4}-\d{2}-\d{2}$/);
});
