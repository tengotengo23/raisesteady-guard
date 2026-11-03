// YouTube page kinds (where our panel goes) and the 3-5 curated watch verdict (content/focus-youtube.js).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const ctx = vm.createContext({});
vm.runInContext(readFileSync(new URL('../../ext/content/focus-youtube.js', import.meta.url), 'utf8'), ctx); // no document: pure part only
const { pageKind, channelKey, curatedVerdict } = ctx.__kgFocusYt;

test('endless grids get our panel; channel home keeps its header; chosen lists get the box above', () => {
  for (const p of ['/', '/feed/trending', '/feed/explore', '/gaming', '/feed/subscriptions', '/feed/subscriptions/']) assert.equal(pageKind(p), 'feed', p);
  for (const p of ['/@SesameStreet', '/@SesameStreet/', '/@SesameStreet/featured', '/channel/UCoookXUzPciGrEZEXmh4Jjg', '/c/sesamestreet', '/user/SesameStreet']) assert.equal(pageKind(p), 'chome', p);
  for (const p of ['/feed/history', '/feed/library', '/feed/you', '/feed/playlists', '/playlist']) assert.equal(pageKind(p), 'above', p);
  for (const p of ['/watch', '/results', '/@SesameStreet/videos', '/@SesameStreet/playlists', '/@SesameStreet/about', '/@SesameStreet/search', '/channel/UCx/videos', '/shorts/abcdefghijk', '/feed/storefront']) assert.equal(pageKind(p), null, p);
});

test('channel keys from owner links', () => {
  assert.equal(channelKey('/@SesameStreet'), 'sesamestreet');
  assert.equal(channelKey('https://www.youtube.com/@Ms%20Rachel/videos'), 'ms rachel');
  assert.equal(channelKey('/channel/UCoookXUzPciGrEZEXmh4Jjg'), 'ucoookxuzpcigrezexmh4jjg');
  assert.equal(channelKey('/watch?v=x'), null);
  assert.equal(channelKey('/@%E0%A4'), null);
});

test('3-5 watch verdict: curated passes, others go to the block page, unknown fails closed', () => {
  const curated = new Set(['ucoookxuzpcigrezexmh4jjg', 'sesamestreet']);
  assert.equal(curatedVerdict({ curated, dataCh: 'UCoookXUzPciGrEZEXmh4Jjg', elapsedMs: 0 }), 'ok');
  assert.equal(curatedVerdict({ curated, dataCh: null, domKeys: ['sesamestreet'], elapsedMs: 0 }), 'ok');
  assert.equal(curatedVerdict({ curated, dataCh: 'UCSMOQeBJ2RAnuFungnQOxLg', elapsedMs: 0 }), 'block');
  assert.equal(curatedVerdict({ curated, dataCh: null, domKeys: ['blenderofficial'], elapsedMs: 500 }), 'wait'); // owner link may still be the last video's
  assert.equal(curatedVerdict({ curated, dataCh: null, domKeys: ['blenderofficial'], elapsedMs: 2000 }), 'block');
  assert.equal(curatedVerdict({ curated, dataCh: null, elapsedMs: 3000 }), 'wait');
  assert.equal(curatedVerdict({ curated, dataCh: null, elapsedMs: 11000 }), 'block');
});
