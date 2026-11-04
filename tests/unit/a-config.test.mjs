// band defaults, config precedence, rules built for each state.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolveConfig, publicConfig, isManaged, pickLang, bandById, DEFAULT_BAND } from '../../ext/background/config.js';
import { buildRules, reasonFor, RULE_ID } from '../../ext/background/guard.js';
import { wantedRulesets } from '../../ext/background/safesearch.js';
import { cleanDomain, inDomains, hostOf } from '../../ext/background/domains.js';

const { bands } = JSON.parse(readFileSync(new URL('../../shared/age-bands.json', import.meta.url), 'utf8'));
const managed = { agentPort: 47615, agentToken: 'a'.repeat(64), ageBand: '6-8', focusMode: true, adblock: true, installId: 'x' };

test('band defaults match the brief table', () => {
  const table = {
    '3-5': [60, 30, '19:30', '07:00', 'strict', 'curated', 'block'],
    '6-8': [90, 45, '20:00', '07:00', 'strict', 'focus', 'block'],
    '9-12': [120, 60, '21:00', '07:00', 'strict', 'focus', 'block'],
    '13-15': [120, 90, '22:00', '06:30', 'moderate', 'focus', 'feedfree'],
    '16-17': [120, 90, '23:00', '06:30', 'moderate', 'focus', 'feedfree'],
  };
  for (const [id, [b, v, s, e, yt, mode, social]] of Object.entries(table)) {
    const c = resolveConfig({ bands, local: { ageBand: id } });
    assert.equal(c.ageBand, id);
    assert.deepEqual(c.limits, { browseMin: b, videoMin: v }, id);
    assert.deepEqual(c.bedtime, { start: s, end: e }, id);
    assert.equal(c.band.youtube, yt); assert.equal(c.band.youtubeMode, mode); assert.equal(c.band.social, social);
  }
});

test('standalone without settings uses the default band and has no agent', () => {
  const c = resolveConfig({ bands, managed: null, local: {} });
  assert.equal(c.standalone, true);
  assert.equal(c.ageBand, DEFAULT_BAND);
  assert.equal(c.agentToken, null);
  assert.equal(c.setupDone, false);
  assert.equal(bandById(bands, 'nope').id, DEFAULT_BAND);
});

test('standalone: local settings beat band defaults, field by field', () => {
  const c = resolveConfig({ bands, local: { ageBand: '6-8', limits: { browseMin: 30 }, bedtime: null, focusMode: false, allow: ['https://www.Example.org/x'], pin: { hash: 'x' } } });
  assert.equal(c.limits.browseMin, 30);
  assert.equal(c.limits.videoMin, 45, 'video falls back to the band');
  assert.equal(c.bedtime, null, 'bedtime switched off');
  assert.equal(c.focusMode, false);
  assert.equal(c.adblock, true);
  assert.deepEqual(c.allow, ['example.org']);
  assert.equal(c.setupDone, true);
  assert.equal(resolveConfig({ bands, local: { ageBand: '6-8', limits: { browseMin: null } } }).limits.browseMin, null, 'null = no limit');
});

test('managed: policy beats local settings; agent state beats policy', () => {
  const local = { ageBand: '16-17', limits: { browseMin: 999 } };
  const m = resolveConfig({ bands, managed, local });
  assert.equal(m.standalone, false);
  assert.equal(m.ageBand, '6-8', 'local settings ignored when managed');
  assert.equal(m.limits.browseMin, 90);
  assert.equal(m.agentPort, 47615);
  const agent = { band: '9-12', limits: { browseMin: 100, videoMin: 50 }, extraMin: 15, bedtime: { start: '21:30', end: '07:00' }, allow: ['khanacademy.org'], block: ['bad.example'], focusMode: false, adblock: false };
  const a = resolveConfig({ bands, managed, local, agent, localExtraMin: 60 });
  assert.equal(a.ageBand, '9-12');
  assert.deepEqual(a.limits, { browseMin: 100, videoMin: 50 });
  assert.equal(a.extraMin, 15, 'agent extra, not the local one');
  assert.deepEqual(a.bedtime, { start: '21:30', end: '07:00' });
  assert.equal(a.focusMode, false); assert.equal(a.adblock, false);
  assert.deepEqual(a.block, ['bad.example']);
  assert.equal(a.band.browseMin, 100, 'band in kg:core:config carries the effective limits');
});

test('managed detection, bad port falls back, token never public', () => {
  assert.equal(isManaged({}), false);
  assert.equal(isManaged({ agentToken: '' }), false);
  assert.equal(isManaged({ ageBand: '6-8' }), true);
  const c = resolveConfig({ bands, managed: { ...managed, agentPort: 99999 } });
  assert.equal(c.agentPort, 47615);
  const pub = publicConfig(c);
  assert.deepEqual(Object.keys(pub).sort(), ['adblock', 'ageBand', 'band', 'focusMode', 'lang', 'standalone', 'topics']);
  assert.equal(JSON.stringify(pub).includes('a'.repeat(64)), false);
});

test('language: ka and ru when the browser uses them, else en', () => {
  assert.equal(pickLang('ka-GE'), 'ka'); assert.equal(pickLang('ru'), 'ru'); assert.equal(pickLang('de-DE'), 'en'); assert.equal(pickLang(undefined), 'en');
});

test('rulesets follow band and switches', () => {
  const young = wantedRulesets(resolveConfig({ bands, local: { ageBand: '6-8' } }));
  assert.equal(young.social, true); assert.equal(young.yt_strict, true); assert.equal(young.yt_moderate, false); assert.equal(young.ads, true);
  const teen = wantedRulesets(resolveConfig({ bands, local: { ageBand: '13-15', adblock: false } }));
  assert.equal(teen.social, false); assert.equal(teen.yt_strict, false); assert.equal(teen.yt_moderate, true); assert.equal(teen.ads, false); assert.equal(teen.yt_ads, false);
  assert.equal(teen.adult && teen.gambling && teen.bypass && teen.safesearch, true);
});

test('dynamic rules: priorities and exclusions', () => {
  const learning = ['khanacademy.org'];
  assert.deepEqual(buildRules({ learning }), []);
  const r = buildRules({ bedtime: true, timeOver: true, videoOver: true, allow: ['ok.example'], block: ['bad.example'], content: ['flagged.example', 'ok.example'], learning });
  const by = Object.fromEntries(r.map((x) => [x.id, x]));
  assert.equal(by[RULE_ID.bedtime].priority, 300);
  assert.ok(by[RULE_ID.bedtime].condition.excludedRequestDomains.includes('127.0.0.1'));
  assert.ok(by[RULE_ID.time].condition.excludedRequestDomains.includes('khanacademy.org'), 'learning sites stay open when time is up');
  assert.ok(by[RULE_ID.time].priority > by[RULE_ID.allow].priority, 'parent allow does not lift the time limit');
  assert.ok(by[RULE_ID.allow].priority > 1, 'parent allow beats the category lists');
  assert.deepEqual(by[RULE_ID.content].condition.requestDomains, ['flagged.example'], 'allowed sites leave the page-check list');
  assert.equal(by[RULE_ID.blockMain].action.redirect.extensionPath, '/pages/blocked.html?reason=parent');
  assert.equal(by[RULE_ID.videoMain].action.redirect.extensionPath, '/pages/blocked.html?reason=video');
  assert.equal(new Set(r.map((x) => x.id)).size, r.length, 'unique ids');
});

test('open tabs: reason for a host mirrors the rules', () => {
  const s = { learning: ['khanacademy.org'], allow: ['ok.example'], block: ['bad.example'], content: ['flagged.example'] };
  assert.equal(reasonFor('www.khanacademy.org'.replace(/^www\./, ''), { ...s, timeOver: true }), null);
  assert.equal(reasonFor('news.example', { ...s, timeOver: true }), 'time');
  assert.equal(reasonFor('khanacademy.org', { ...s, bedtime: true }), 'bedtime', 'bedtime applies to learning sites too');
  assert.equal(reasonFor('127.0.0.1', { ...s, bedtime: true }), null, 'the parent page stays open');
  assert.equal(reasonFor('m.youtube.com', { ...s, videoOver: true }), 'video');
  assert.equal(reasonFor('sub.bad.example', s), 'parent');
  assert.equal(reasonFor('flagged.example', s), 'content');
  assert.equal(reasonFor('ok.example', s), null);
});

test('domain helpers', () => {
  assert.equal(cleanDomain(' HTTPS://www.Khanacademy.org/math '), 'khanacademy.org');
  assert.equal(cleanDomain('not a domain'), null);
  assert.equal(cleanDomain('localhost'), null);
  assert.equal(hostOf('chrome-extension://abc/pages/x.html'), null);
  assert.equal(inDomains('a.b.example.org', ['example.org']), true);
  assert.equal(inDomains('badexample.org', ['example.org']), false);
});

test('managed booleans given as 1/0 (Firefox on Windows) are read as booleans', () => {
  const bands = JSON.parse(readFileSync(new URL('../../shared/age-bands.json', import.meta.url), 'utf8')).bands;
  const off = resolveConfig({ bands, managed: { agentToken: 'a'.repeat(64), ageBand: '9-12', focusMode: 0, adblock: 0 }, local: {}, agent: null, uiLang: 'en' });
  assert.equal(off.focusMode, false);
  assert.equal(off.adblock, false);
  const on = resolveConfig({ bands, managed: { agentToken: 'a'.repeat(64), ageBand: '9-12', focusMode: 1, adblock: '1' }, local: {}, agent: null, uiLang: 'en' });
  assert.equal(on.focusMode, true);
  assert.equal(on.adblock, true);
});

// ---- review fixes: page-level page-check memory, learning paths ----
import { contentScope } from '../../ext/background/guard.js';
import { parseLearning, isLearningUrl, isMixedHost, urlKey } from '../../ext/background/domains.js';

const learningFile = JSON.parse(readFileSync(new URL('../../shared/learning-domains.json', import.meta.url), 'utf8')).domains;
const L = parseLearning(learningFile);

test('page check: label blocks the host, words block the page only', () => {
  assert.equal(contentScope('badsite.example', 'label', L), 'host');
  assert.equal(contentScope('badsite.example', 'rating', L), 'host');
  assert.equal(contentScope('badsite.example', 'words', L), 'url');
  for (const h of ['en.wikipedia.org', 'www.google.com', 'google.co.uk', 'm.youtube.com', 'bing.com', 'duckduckgo.com', 'web.archive.org', 'github.com', 'old.reddit.com', 'medium.com', 'yandex.ru']) {
    assert.equal(contentScope(h, 'label', L), 'url', `${h} is never blocked as a whole`);
  }
  assert.equal(contentScope('www.khanacademy.org'.replace('www.', ''), 'label', L), 'url', 'learning host');
  assert.equal(contentScope('www.bbc.co.uk', 'label', L), 'url', 'host of a learning path');
  assert.equal(isMixedHost('notgoogle.com'), false);
  assert.equal(isMixedHost('google.evil.example'), false);
});

test('page-level rules block that page only, and reasonFor follows', () => {
  const s = { contentUrls: [urlKey('https://en.wikipedia.org/wiki/Some_page?x=1#y'), 'https://a.example/odd*path'], allow: [], learning: [] };
  assert.equal(s.contentUrls[0], 'https://en.wikipedia.org/wiki/Some_page');
  const r = buildRules(s);
  assert.equal(r.length, 1, 'URLs with urlFilter special characters are left to the page check');
  assert.equal(r[0].condition.urlFilter, '|https://en.wikipedia.org/wiki/Some_page^');
  assert.equal(r[0].action.redirect.extensionPath, '/pages/blocked.html?reason=content');
  assert.equal(reasonFor('en.wikipedia.org', s, 'https://en.wikipedia.org/wiki/Some_page?y=2'), 'content');
  assert.equal(reasonFor('en.wikipedia.org', s, 'https://en.wikipedia.org/wiki/Cats'), null, 'the rest of the host stays open');
  assert.equal(buildRules({ ...s, allow: ['wikipedia.org'] }).length, 1, 'allow rule only');
});

test('learning list: broad hosts out, learning paths in, bookmarks still learning', () => {
  for (const d of ['bbc.co.uk', 'bbc.com', 'pbs.org', 'nationalgeographic.com', 'ted.com']) assert.ok(!learningFile.includes(d), d);
  assert.equal(isLearningUrl('https://www.bbc.co.uk/bitesize/subjects/z123', L), true);
  assert.equal(isLearningUrl('https://www.bbc.co.uk/cbeebies', L), true);
  assert.equal(isLearningUrl('https://www.bbc.co.uk/iplayer/episode/x', L), false);
  assert.equal(isLearningUrl('https://www.bbc.co.uk/bitesizer', L), false);
  assert.equal(isLearningUrl('https://www.ted.com/talks/x', L), false);
  assert.equal(isLearningUrl('https://ed.ted.com/lessons/x', L), true);
  assert.equal(isLearningUrl('https://www.pbs.org/video/x', L), false);
  assert.equal(isLearningUrl('https://pbskids.org/games', L), true);
  assert.equal(isLearningUrl('https://kids.nationalgeographic.com/animals', L), true);
  const bookmarks = JSON.parse(readFileSync(new URL('../../shared/bookmarks.json', import.meta.url), 'utf8'));
  const urls = bookmarks.folders.flatMap((f) => f.items.map((i) => i.url));
  for (const u of ['https://www.bbc.co.uk/bitesize', 'https://www.bbc.co.uk/cbeebies', 'https://ed.ted.com/', 'https://kids.nationalgeographic.com/']) {
    if (urls.includes(u)) assert.equal(isLearningUrl(u, L), true, u);
  }
  const t = buildRules({ timeOver: true, learning: L.hosts, learningPaths: L.paths });
  const pathRules = t.filter((x) => x.action.type === 'allow');
  assert.ok(pathRules.some((x) => x.condition.urlFilter === '||bbc.co.uk/bitesize' && x.priority > 200), 'bitesize stays open when time is up');
  assert.equal(reasonFor('bbc.co.uk', { timeOver: true, learning: L.hosts, learningPaths: L.paths }, 'https://www.bbc.co.uk/bitesize/x'), null);
  assert.equal(reasonFor('bbc.co.uk', { timeOver: true, learning: L.hosts, learningPaths: L.paths }, 'https://www.bbc.co.uk/iplayer'), 'time');
  assert.equal(buildRules({ bedtime: true, learning: L.hosts, learningPaths: L.paths }).some((x) => x.action.type === 'allow'), false, 'bedtime has no learning exception');
});
