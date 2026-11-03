// YouTube ad-field pruning (content/yt-ads-main.js) on saved real player and watch data.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const src = readFileSync(new URL('../../ext/content/yt-ads-main.js', import.meta.url), 'utf8');
const ctx = vm.createContext({});
vm.runInContext(src, ctx); // no document: the script only exposes its pure part
const yt = ctx.__kgYtAds;
const FIX = new URL('../e2e/fixtures-b/', import.meta.url);
const load = (f) => JSON.parse(readFileSync(new URL(f, FIX), 'utf8'));
const has = (o, key) => JSON.stringify(o).includes(`"${key}"`);

test('exposes only the pure part without a document (no hooks installed in node)', () => {
  assert.equal(typeof yt.pruneResponse, 'function');
  assert.equal(ctx.__kgYtAdsOn, undefined);
});

test('real player response (saved 3 Nov 2026): ad fields go, playback data stays', () => {
  const p = load('yt-player.json');
  assert.ok(has(p, 'adPlacements') && has(p, 'adBreakHeartbeatParams'));
  const before = { streaming: JSON.stringify(p.streamingData), details: JSON.stringify(p.videoDetails), play: JSON.stringify(p.playabilityStatus) };
  assert.equal(yt.pruneResponse(p, { ads: true, focus: false }), true);
  assert.ok(!has(p, 'adPlacements') && !has(p, 'adBreakHeartbeatParams') && !has(p, 'playerAds') && !has(p, 'adSlots'));
  assert.equal(JSON.stringify(p.streamingData), before.streaming);
  assert.equal(JSON.stringify(p.videoDetails), before.details);
  assert.equal(JSON.stringify(p.playabilityStatus), before.play);
  assert.ok(p.endscreen, 'end screen kept while focus is off');
  assert.equal(yt.pruneResponse(p, { ads: true, focus: false }), false, 'second pass finds nothing');
  assert.equal(yt.pruneResponse(p, { ads: false, focus: true }), true);
  assert.equal(p.endscreen, undefined);
});

test('real watch-next data: ad panel, autoplay and end-screen wall removed only when asked', () => {
  const n = load('yt-next.json');
  const panels = () => n.engagementPanels.map((e) => e.engagementPanelSectionListRenderer && e.engagementPanelSectionListRenderer.targetId);
  assert.ok(panels().includes('engagement-panel-ads'));
  const nPanels = n.engagementPanels.length;
  yt.pruneResponse(n, { ads: false, focus: false });
  assert.equal(n.engagementPanels.length, nPanels, 'nothing happens with both switches off');
  yt.pruneResponse(n, { ads: true, focus: false });
  assert.ok(!panels().includes('engagement-panel-ads'));
  assert.equal(n.engagementPanels.length, nPanels - 1);
  assert.ok(n.contents.twoColumnWatchNextResults.autoplay && n.playerOverlays.playerOverlayRenderer.endScreen);
  yt.pruneResponse(n, { ads: true, focus: true });
  assert.equal(n.contents.twoColumnWatchNextResults.autoplay, undefined);
  assert.equal(n.playerOverlays.playerOverlayRenderer.autoplay, undefined);
  assert.equal(n.playerOverlays.playerOverlayRenderer.endScreen, undefined);
  assert.ok(n.contents.twoColumnWatchNextResults.results, 'the video and its description stay');
});

test('feed and search ads: wrapped ad items leave the list, real items stay', () => {
  const browse = { responseContext: {}, contents: { richGridRenderer: { contents: [
    { richItemRenderer: { content: { videoRenderer: { videoId: 'aaaaaaaaaaa' } } } },
    { richItemRenderer: { content: { adSlotRenderer: { x: 1 } } } },
    { richSectionRenderer: { content: { statementBannerRenderer: {} } } },
    { richItemRenderer: { content: { videoRenderer: { videoId: 'bbbbbbbbbbb' } } } },
  ] } } };
  assert.equal(yt.pruneResponse(browse, { ads: true, focus: false }), true);
  assert.deepEqual(browse.contents.richGridRenderer.contents.map((c) => c.richItemRenderer.content.videoRenderer.videoId), ['aaaaaaaaaaa', 'bbbbbbbbbbb']);
  const search = [{ playerResponse: { adPlacements: [1], videoDetails: {} } }, { response: { responseContext: {}, x: [{ searchPyvRenderer: {} }, { videoRenderer: {} }] } }];
  assert.ok(yt.looksInnertube(search));
  yt.pruneResponse(search, { ads: true, focus: false });
  assert.equal(search[0].playerResponse.adPlacements, undefined);
  assert.equal(search[1].response.x.length, 1);
  assert.equal(yt.isAdItem({ engagementPanelSectionListRenderer: { targetId: 'engagement-panel-searchable-transcript' } }), false);
  assert.equal(yt.looksInnertube({ foo: 1 }), false);
  const deep = { responseContext: {} }; let cur = deep; for (let i = 0; i < 200; i++) { cur.a = { adSlots: [] }; cur = cur.a; }
  assert.doesNotThrow(() => yt.pruneResponse(deep, { ads: true, focus: true })); // depth-capped
});
