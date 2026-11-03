// YouTube ad removal in the page's own world (our own code, not derived from any GPL tool).
// Runs at document_start in the MAIN world on youtube.com. It removes ad fields from YouTube's data
// before the player reads them: the initial page data (ytInitialPlayerResponse, ytInitialData),
// JSON.parse results, and fetch / XHR answers of the youtubei player, next, browse, search and reel endpoints.
// With focus mode on it also removes the end-screen video wall and the autoplay "up next" data.
// Fallback: if an ad still starts, it is muted, run to its end and skipped.
// Switches: <html data-kg-ads="off"> (content/cosmetic.js) and <html data-kg-focus="off"> (content/focus-youtube.js).
// Fails safe: every hook catches its own errors and hands YouTube the original data.
(function (root) {
  'use strict';

  // ---- pure part (unit-tested in node through globalThis.__kgYtAds when there is no document) ----
  const PLAYER_AD_KEYS = ['adPlacements', 'playerAds', 'adSlots', 'adBreakHeartbeatParams', 'adBreakParams'];
  const AD_RENDERERS = new Set([
    'adSlotRenderer', 'promotedSparklesWebRenderer', 'promotedSparklesTextSearchRenderer', 'promotedVideoRenderer',
    'compactPromotedVideoRenderer', 'searchPyvRenderer', 'bannerPromoRenderer', 'statementBannerRenderer',
    'brandVideoSingletonRenderer', 'brandVideoShelfRenderer', 'inFeedAdLayoutRenderer', 'actionCompanionAdRenderer',
    'playerLegacyDesktopWatchAdsRenderer', 'adBreakServiceRenderer', 'merchandiseShelfRenderer', 'compactPromotedItemRenderer',
  ]);
  const WRAPPERS = ['richItemRenderer', 'richSectionRenderer'];
  const MAX_DEPTH = 60;

  function hasAdKey(o) {
    if (!o || typeof o !== 'object') return false;
    for (const k in o) if (AD_RENDERERS.has(k)) return true;
    return false;
  }

  /** An entry of a renderer list that is (or only wraps) an ad. */
  function isAdItem(x) {
    if (!x || typeof x !== 'object' || Array.isArray(x)) return false;
    if (hasAdKey(x)) return true;
    for (const w of WRAPPERS) if (x[w] && x[w].content && hasAdKey(x[w].content)) return true;
    const ep = x.engagementPanelSectionListRenderer;
    if (ep && typeof ep.targetId === 'string' && /(^|-)ads(-|$)/.test(ep.targetId)) return true;
    return false;
  }

  /** Remove ad entries and ad keys everywhere in o (in place). Returns true when something changed. */
  function pruneAdsDeep(o, depth) {
    if (!o || typeof o !== 'object' || depth > MAX_DEPTH) return false;
    let changed = false;
    if (Array.isArray(o)) {
      for (let i = o.length - 1; i >= 0; i--) {
        if (isAdItem(o[i])) { o.splice(i, 1); changed = true; } else if (pruneAdsDeep(o[i], depth + 1)) changed = true;
      }
      return changed;
    }
    for (const k of PLAYER_AD_KEYS) if (k in o) { delete o[k]; changed = true; }
    for (const k in o) {
      const v = o[k];
      if (v && typeof v === 'object') {
        if (AD_RENDERERS.has(k)) { delete o[k]; changed = true; } else if (pruneAdsDeep(v, depth + 1)) changed = true;
      }
    }
    return changed;
  }

  /** Focus mode: no end-screen wall, no autoplay "up next" (player and next responses). */
  function pruneFocus(o) {
    if (!o || typeof o !== 'object') return false;
    let changed = false;
    const del = (obj, key) => { if (obj && typeof obj === 'object' && key in obj) { delete obj[key]; changed = true; } };
    if (o.playabilityStatus || o.videoDetails) del(o, 'endscreen');
    const c = o.contents || {};
    del(c.twoColumnWatchNextResults, 'autoplay');
    del(c.singleColumnWatchNextResults, 'autoplay');
    const po = o.playerOverlays && o.playerOverlays.playerOverlayRenderer;
    del(po, 'autoplay');
    del(po, 'endScreen');
    return changed;
  }

  /** One innertube answer (or a wrapper / list of them). flags = { ads, focus }. Returns true when changed. */
  function pruneResponse(o, flags) {
    if (!o || typeof o !== 'object') return false;
    let changed = false;
    if (Array.isArray(o)) { for (const x of o) if (pruneResponse(x, flags)) changed = true; return changed; }
    for (const k of ['playerResponse', 'response']) if (o[k] && typeof o[k] === 'object' && pruneResponse(o[k], flags)) changed = true;
    if (flags.ads && pruneAdsDeep(o, 0)) changed = true;
    if (flags.focus && pruneFocus(o)) changed = true;
    return changed;
  }

  const looksInnertube = (o) => !!o && typeof o === 'object' && (
    'responseContext' in o || 'playabilityStatus' in o || 'adPlacements' in o || 'playerResponse' in o ||
    (Array.isArray(o) && o.length < 8 && o.some((x) => x && typeof x === 'object' && ('playerResponse' in x || 'response' in x))));

  const core = { pruneResponse, pruneAdsDeep, pruneFocus, isAdItem, looksInnertube };
  if (typeof root.document === 'undefined') { root.__kgYtAds = core; return; }

  // ---- install (browser) ----
  try {
    if (root.__kgYtAdsOn) return;
    Object.defineProperty(root, '__kgYtAdsOn', { value: true });
  } catch { return; }
  const de = () => root.document.documentElement;
  const flags = () => {
    const h = de();
    return { ads: !h || h.getAttribute('data-kg-ads') !== 'off', focus: !h || h.getAttribute('data-kg-focus') !== 'off' };
  };
  const anyOn = (f) => f.ads || f.focus;
  // For focus mode's curated check (3-5): which channel each video belongs to, from YouTube's own player
  // data, written to <html data-kg-vc='{"<videoId>":"<channelId>"}'> (last 12) for content/focus-youtube.js.
  const seenCh = new Map();
  const noteChannel = (o) => {
    try {
      const list = Array.isArray(o) ? o : [o, o && o.playerResponse];
      for (const x of list) {
        const d = x && (x.videoDetails || (x.playerResponse && x.playerResponse.videoDetails));
        if (!d || typeof d.videoId !== 'string' || typeof d.channelId !== 'string') continue;
        if (seenCh.get(d.videoId) === d.channelId) continue;
        seenCh.delete(d.videoId); seenCh.set(d.videoId, d.channelId);
        while (seenCh.size > 12) seenCh.delete(seenCh.keys().next().value);
        const h = de();
        if (h) h.setAttribute('data-kg-vc', JSON.stringify(Object.fromEntries(seenCh)));
      }
    } catch { /* only a helper */ }
  };
  const prune = (o, f) => { noteChannel(o); return pruneResponse(o, f); };
  const safePrune = (o) => { try { const f = flags(); if (anyOn(f) && looksInnertube(o)) return prune(o, f); } catch { /* keep data */ } return false; };
  const ENDPOINT = /\/youtubei\/v1\/(player|next|browse|search|reel\/)/;

  // 1. initial page data
  for (const name of ['ytInitialPlayerResponse', 'ytInitialData']) {
    try {
      const own = Object.getOwnPropertyDescriptor(root, name);
      if (own && !own.configurable) continue;
      let value = own ? own.value : undefined;
      if (value) safePrune(value);
      Object.defineProperty(root, name, {
        configurable: true, enumerable: true,
        get() { return value; },
        set(v) { safePrune(v); value = v; },
      });
    } catch { /* leave it */ }
  }

  // 2. JSON.parse
  const origParse = JSON.parse;
  try {
    const wrapped = function parse(text, reviver) {
      const r = origParse.apply(this, arguments);
      if (r && typeof r === 'object' && typeof text === 'string' && text.length > 200) safePrune(r);
      return r;
    };
    Object.defineProperty(wrapped, 'toString', { value: () => origParse.toString() });
    JSON.parse = wrapped;
  } catch { /* leave it */ }

  // 3. fetch
  const origFetch = root.fetch;
  if (typeof origFetch === 'function') {
    try {
      root.fetch = function fetch(input, init) {
        const p = origFetch.apply(this, arguments);
        let url = '';
        try { url = typeof input === 'string' ? input : (input && input.url) || String(input); } catch { return p; }
        if (!ENDPOINT.test(url)) return p;
        return p.then(async (res) => {
          try {
            const f = flags();
            if (!anyOn(f) || !res || !res.ok || !/json/.test(res.headers.get('content-type') || '')) return res;
            const obj = origParse(await res.clone().text());
            if (!prune(obj, f)) return res;
            const headers = new Headers(res.headers);
            headers.delete('content-length'); headers.delete('content-encoding');
            const out = new Response(JSON.stringify(obj), { status: res.status, statusText: res.statusText, headers });
            try { Object.defineProperty(out, 'url', { value: res.url }); } catch { /* fine */ }
            return out;
          } catch { return res; }
        });
      };
    } catch { /* leave it */ }
  }

  // 4. XMLHttpRequest
  try {
    const XP = root.XMLHttpRequest && root.XMLHttpRequest.prototype;
    const open = XP && XP.open;
    if (open) {
      const urls = new WeakMap(), cache = new WeakMap();
      XP.open = function (method, url) { try { urls.set(this, String(url)); } catch { /* fine */ } return open.apply(this, arguments); };
      for (const prop of ['responseText', 'response']) {
        const d = Object.getOwnPropertyDescriptor(XP, prop);
        if (!d || !d.get || !d.configurable) continue;
        Object.defineProperty(XP, prop, {
          configurable: true, enumerable: d.enumerable,
          get() {
            const v = d.get.call(this);
            try {
              if (this.readyState !== 4 || !ENDPOINT.test(urls.get(this) || '')) return v;
              const c = cache.get(this);
              if (c && c[prop] && c[prop].src === v) return c[prop].out;
              let out = v;
              if (typeof v === 'string' && v) {
                const obj = origParse(v);
                const f = flags();
                if (anyOn(f) && prune(obj, f)) out = JSON.stringify(obj);
              } else if (v && typeof v === 'object') safePrune(v);
              cache.set(this, { ...(c || {}), [prop]: { src: v, out } });
              return out;
            } catch { return v; }
          },
        });
      }
    }
  } catch { /* leave it */ }

  // 5. fallback: an ad that still plays is muted, run to its end and skipped; sound comes back after it.
  let adState = null; // { video, muted }
  const SKIP = '.ytp-skip-ad-button, .ytp-ad-skip-button, .ytp-ad-skip-button-modern, .ytp-ad-skip-button-slot button';
  setInterval(() => {
    try {
      const doc = root.document;
      const player = doc.querySelector('.html5-video-player.ad-showing');
      if (player && flags().ads) {
        const v = player.querySelector('video');
        if (v) {
          if (!adState || adState.video !== v) adState = { video: v, muted: v.muted };
          v.muted = true;
          if (Number.isFinite(v.duration) && v.duration > 0 && v.currentTime < v.duration - 0.25) v.currentTime = Math.max(0, v.duration - 0.1);
        }
        const btn = player.querySelector(SKIP);
        if (btn) btn.click();
      } else if (adState) {
        try { adState.video.muted = adState.muted; } catch { /* gone */ }
        adState = null;
      }
    } catch { /* never break playback */ }
  }, 400);
})(typeof window !== 'undefined' ? window : globalThis);
