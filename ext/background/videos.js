// Curated videos and focus mode data. No API key: the public YouTube RSS feed of each channel
// (https://www.youtube.com/feeds/videos.xml?channel_id=<id>), cached 6 hours in storage.local, merged with
// the built-in list data/videos.json (tools/update-videos.mjs; P2-161), which is used alone when the feeds
// do not answer (they answer 404 on and off). Titles on the topic filter's list are left out.
// Answers kg:focus:* messages:
//   kg:focus:videos   { ageBand, lang, limit }  -> { videos: [{ id, title, channel, channelId, published }], offline, stale, saved }
//     saved = every video shown comes from the built-in list; offline = the browser says it is offline and nothing is shown
//   kg:focus:channels { ageBand }               -> { channels: [{ id, handle, name }] }  (curated search, 3-5)
//   kg:focus:i18n     { lang }                  -> { strings }                            (i18n/focus.<lang>.json over en)
//   kg:focus:state                              -> { focusMode, ageBand, youtubeMode, kids, social, lang }
// Also writes kg.b.focus to storage.local so content scripts know the mode at document_start without waking us.

export const FEED_TTL_MS = 6 * 60 * 60 * 1000;
export const RETRY_MS = 30 * 60 * 1000;
const FETCH_TIMEOUT_MS = 10000;
const CONCURRENCY = 3;
const SPACING_MS = 250;
const MAX_SHORTS_CACHE = 3000;
const K = { feeds: 'kg.b.feeds', shorts: 'kg.b.shorts', focus: 'kg.b.focus' };
const ALARM = 'kg-b-videos';
const ID_RE = /^[\w-]{11}$/;
const CH_RE = /^UC[\w-]{22}$/;

// ---- pure parts (unit-tested) ----

const ENT = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ' };
export function decodeEntities(s) {
  return String(s ?? '').replace(/&(#x[0-9a-f]+|#\d+|[a-z]+);/gi, (m, e) => {
    if (e[0] === '#') {
      const n = e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10);
      return Number.isFinite(n) && n > 0 && n < 0x110000 && !(n >= 0xd800 && n < 0xe000) ? String.fromCodePoint(n) : '';
    }
    return Object.prototype.hasOwnProperty.call(ENT, e.toLowerCase()) ? ENT[e.toLowerCase()] : m;
  });
}

const textOf = (xml, tag) => {
  const m = new RegExp(`<${tag}(?:\\s[^>]*)?>([\\s\\S]*?)</${tag}>`).exec(xml);
  if (!m) return '';
  let v = m[1].trim();
  const cdata = /^<!\[CDATA\[([\s\S]*?)\]\]>$/.exec(v);
  v = cdata ? cdata[1] : decodeEntities(v.replace(/<[^>]*>/g, ''));
  return v.replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, '').replace(/\s+/g, ' ').trim();
};
const attrOf = (tagText, attr) => {
  const m = new RegExp(`\\s${attr}="([^"]*)"`).exec(tagText);
  return m ? decodeEntities(m[1]) : '';
};

/**
 * Parse a YouTube channel Atom feed without DOMParser (service workers have none).
 * Only reads the fields we need; everything is treated as text. Returns null for something that is not a feed.
 */
export function parseFeed(xml) {
  if (typeof xml !== 'string' || !/<feed[\s>]/.test(xml)) return null;
  const head = xml.split('<entry>')[0];
  const channelId = textOf(head, 'yt:channelId');
  const out = { channelId: channelId && !channelId.startsWith('UC') ? `UC${channelId}` : channelId, title: textOf(head, 'title'), videos: [] };
  const entries = xml.match(/<entry>[\s\S]*?<\/entry>/g) || [];
  for (const e of entries) {
    const id = textOf(e, 'yt:videoId');
    if (!ID_RE.test(id)) continue;
    const linkTag = (/<link\s[^>]*rel="alternate"[^>]*>/.exec(e) || [''])[0];
    const href = attrOf(linkTag, 'href');
    const author = (/<author>([\s\S]*?)<\/author>/.exec(e) || ['', ''])[1];
    const published = textOf(e, 'published');
    const ts = Date.parse(published);
    out.videos.push({
      id,
      title: textOf(e, 'title').slice(0, 200),
      channel: textOf(author, 'name').slice(0, 100) || out.title,
      channelId: textOf(e, 'yt:channelId') || out.channelId,
      published: Number.isFinite(ts) ? ts : 0,
      short: shortFromLink(href),
    });
  }
  return out;
}

/** true = Short, false = normal video, null = cannot tell from the feed link. */
export function shortFromLink(href) {
  if (typeof href !== 'string' || !href) return null;
  if (/^https:\/\/(www\.|m\.)?youtube\.com\/shorts\/[\w-]{11}/.test(href)) return true;
  if (/^https:\/\/(www\.|m\.)?youtube\.com\/watch\?v=[\w-]{11}/.test(href)) return false;
  return null;
}

/** Drop Shorts. `known` = cached answers by video id; unknown entries are dropped too (we never guess "not a Short"). */
export function dropShorts(videos, known = {}) {
  return (videos || []).filter((v) => {
    const s = v.short ?? known[v.id];
    return s === false;
  });
}

/**
 * Mix channels so one busy channel cannot fill the grid: round by round, the newest unused video of each
 * channel; within a round, channels in the wanted language first, then newest first.
 * lists: [{ channelId, lang, videos }] with videos in any order.
 */
export function mixVideos(lists, { limit = 12, lang = 'en', maxAgeDays = 0, now = Date.now() } = {}) {
  const queues = [];
  for (const l of lists || []) {
    let vids = [...(l.videos || [])].filter((v) => v && ID_RE.test(v.id));
    if (maxAgeDays > 0) vids = vids.filter((v) => !v.published || now - v.published <= maxAgeDays * 86400000);
    vids.sort((a, b) => (b.published || 0) - (a.published || 0));
    if (vids.length) queues.push({ match: l.lang === lang, vids });
  }
  const out = [], seen = new Set();
  while (out.length < limit && queues.some((q) => q.vids.length)) {
    const round = queues.filter((q) => q.vids.length)
      .sort((a, b) => (b.match - a.match) || ((b.vids[0].published || 0) - (a.vids[0].published || 0)));
    for (const q of round) {
      if (out.length >= limit) break;
      while (q.vids.length) {
        const v = q.vids.shift();
        if (seen.has(v.id)) continue;
        seen.add(v.id); out.push(v); break;
      }
    }
  }
  return out;
}

/** The curated channels for an age band and language: the language's own channels plus English ones. */
export function channelsFor(channels, ageBand, lang) {
  const langs = new Set([lang, 'en']);
  return (channels || []).filter((c) => c && CH_RE.test(c.id) && Array.isArray(c.bands) && c.bands.includes(ageBand) && langs.has(c.lang || 'en'));
}

export const BUILTIN_MAX_AGE_DAYS = 120;
/** Days since the built-in list was made ('YYYY-MM-DD'), or Infinity when unknown. Store builds refuse an old list. */
export function builtinAgeDays(built, now = Date.now()) {
  const ms = /^\d{4}-\d{2}-\d{2}$/.test(built || '') ? Date.parse(`${built}T00:00:00Z`) : NaN;
  return Number.isFinite(ms) ? Math.floor((now - ms) / 86400000) : Infinity;
}

/** A built-in video ({ id, title, channel, published: 'YYYY-MM-DD' }) in the feed's shape. */
export function builtinVideo(v, channelId) {
  if (!v || !ID_RE.test(v.id) || typeof v.title !== 'string') return null;
  const ts = /^\d{4}-\d{2}-\d{2}$/.test(v.published || '') ? Date.parse(`${v.published}T00:00:00Z`) : 0;
  return { id: v.id, title: String(v.title).slice(0, 200), channel: String(v.channel || '').slice(0, 100), channelId, published: Number.isFinite(ts) ? ts : 0 };
}

/**
 * One list per channel: the feed's videos (fresh or cached) and the built-in ones, without doubles
 * (the feed's copy wins). Each video carries src 'feed' or 'saved'. Titles the topic filter finds are dropped.
 * channels: [{ id, lang }]; feeds: { id: { videos } }; builtin: { id: [video] }; topics: matcher or null.
 */
export function mergeLists(channels, feeds, builtin, topics = null) {
  const ok = (v) => !topics || !topics.matchTitle(`${v.title} ${v.channel || ''}`).hit;
  return (channels || []).map((c) => {
    const seen = new Set(), videos = [];
    for (const v of ((feeds || {})[c.id] && feeds[c.id].videos) || []) {
      if (v && ID_RE.test(v.id) && !seen.has(v.id) && ok(v)) { seen.add(v.id); videos.push({ ...v, src: 'feed' }); }
    }
    for (const raw of ((builtin || {})[c.id]) || []) {
      const v = builtinVideo(raw, c.id);
      if (v && !seen.has(v.id) && ok(v)) { seen.add(v.id); videos.push({ ...v, src: 'saved' }); }
    }
    return { channelId: c.id, lang: c.lang || 'en', videos };
  });
}

/** What the grid shows: 'grid', 'grid-saved' (with the "Showing saved videos" line), 'offline' or 'empty'. */
export function gridState(res, online) {
  const n = res && Array.isArray(res.videos) ? res.videos.length : 0;
  if (n) return res.saved ? 'grid-saved' : 'grid';
  return online === false ? 'offline' : 'empty';
}

/** Small parallel queue: at most `n` jobs at a time, started `spacing` ms apart. */
export async function pool(items, n, spacing, job) {
  const results = new Array(items.length);
  let next = 0;
  const worker = async (w) => {
    if (spacing) await new Promise((r) => setTimeout(r, w * spacing));
    while (next < items.length) {
      const i = next++;
      try { results[i] = await job(items[i], i); } catch (e) { results[i] = undefined; }
      if (spacing && next < items.length) await new Promise((r) => setTimeout(r, spacing));
    }
  };
  await Promise.all(Array.from({ length: Math.min(n, items.length) }, (_, w) => worker(w)));
  return results;
}

// ---- runtime ----

export function initVideos(ctx) {
  const api = ctx.api;
  let channels = null, builtin = null, feeds = null, shorts = null, i18nCache = new Map();
  const inflight = new Map();
  let saveTimer = null;

  const loadData = async () => {
    if (!channels) {
      try { channels = ((await (await fetch(api.runtime.getURL('data/channels.json'))).json()) || {}).channels || []; } catch { channels = []; }
    }
    if (!builtin) {
      try { builtin = ((await (await fetch(api.runtime.getURL('data/videos.json'))).json()) || {}).channels || {}; } catch { builtin = {}; }
    }
    if (!feeds) {
      const st = await api.storage.local.get([K.feeds, K.shorts]).catch(() => ({}));
      feeds = st[K.feeds] || {};
      shorts = st[K.shorts] || {};
    }
  };
  const save = () => {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      const ids = Object.keys(shorts);
      if (ids.length > MAX_SHORTS_CACHE) for (const id of ids.slice(0, ids.length - MAX_SHORTS_CACHE)) delete shorts[id];
      api.storage.local.set({ [K.feeds]: feeds, [K.shorts]: shorts }).catch(() => {});
    }, 400);
  };

  const timedFetch = async (url, opts = {}) => {
    const ac = new AbortController();
    const t = setTimeout(() => ac.abort(), FETCH_TIMEOUT_MS);
    try { return await fetch(url, { credentials: 'omit', cache: 'no-store', ...opts, signal: ac.signal }); } finally { clearTimeout(t); }
  };

  // When a feed entry does not say, ask youtube.com once: /shorts/<id> answers 200 for a Short and
  // redirects to /watch for a normal video. The answer is cached for good.
  const probeShort = async (id) => {
    try {
      const r = await timedFetch(`https://www.youtube.com/shorts/${id}`, { method: 'HEAD', redirect: 'manual' });
      if (r.type === 'opaqueredirect' || (r.status >= 300 && r.status < 400)) return false;
      if (r.status === 200 && !r.redirected) return true;
    } catch { /* offline */ }
    return null;
  };

  const fetchChannel = async (id) => {
    if (inflight.has(id)) return inflight.get(id);
    const p = (async () => {
      const prev = feeds[id];
      try {
        const r = await timedFetch(`https://www.youtube.com/feeds/videos.xml?channel_id=${encodeURIComponent(id)}`);
        if (!r.ok) throw new Error(`feed ${r.status}`);
        const feed = parseFeed(await r.text());
        if (!feed) throw new Error('not a feed');
        for (const v of feed.videos) {
          if (v.short === null && shorts[v.id] === undefined) {
            const s = await probeShort(v.id);
            if (s !== null) shorts[v.id] = s;
          } else if (v.short !== null) shorts[v.id] = v.short;
        }
        const videos = dropShorts(feed.videos, shorts).map(({ short, ...v }) => v);
        feeds[id] = { at: Date.now(), videos };
      } catch {
        feeds[id] = { ...(prev || { videos: [] }), at: prev ? prev.at : 0, failAt: Date.now() };
      }
      save();
      return feeds[id];
    })();
    inflight.set(id, p);
    try { return await p; } finally { inflight.delete(id); }
  };

  const fresh = (f, now) => f && f.at && now - f.at < FEED_TTL_MS;
  const retryable = (f, now) => !f || !f.failAt || now - f.failAt > RETRY_MS;

  async function videosFor({ ageBand, lang = 'en', limit = 12 } = {}) {
    await loadData();
    const cfg = ctx.getConfig();
    const band = typeof ageBand === 'string' && ageBand ? ageBand : (cfg && cfg.ageBand) || '9-12';
    const list = channelsFor(channels, band, String(lang || 'en'));
    const now = Date.now();
    const stale = list.filter((c) => !fresh(feeds[c.id], now) && retryable(feeds[c.id], now));
    if (stale.length) await pool(stale, CONCURRENCY, SPACING_MS, (c) => fetchChannel(c.id));
    const lists = mergeLists(list, feeds, builtin, typeof ctx.topics === 'function' ? ctx.topics() : null);
    const n = Math.max(1, Math.min(48, Number(limit) || 12));
    const mixed = mixVideos(lists, { limit: n, lang: String(lang || 'en') });
    const anyFresh = list.some((c) => fresh(feeds[c.id], Date.now()));
    const saved = mixed.length > 0 && mixed.every((v) => v.src === 'saved');
    const videos = mixed.map(({ src, ...v }) => v);
    const online = globalThis.navigator ? navigator.onLine !== false : true;
    return { videos, offline: !videos.length && !online, stale: !anyFresh, saved };
  }

  async function strings(lang) {
    const l = ['en', 'ka', 'ru', 'es', 'zh'].includes(lang) ? lang : 'en';
    if (i18nCache.has(l)) return i18nCache.get(l);
    const get = async (x) => { try { const r = await fetch(api.runtime.getURL(`i18n/focus.${x}.json`)); return r.ok ? await r.json() : {}; } catch { return {}; } };
    const dict = { ...(await get('en')), ...(l === 'en' ? {} : await get(l)) };
    i18nCache.set(l, dict);
    return dict;
  }

  const focusState = (cfg) => {
    if (!cfg) return null;
    const b = cfg.band || {};
    return {
      focusMode: cfg.focusMode !== false, ageBand: cfg.ageBand, youtubeMode: b.youtubeMode || 'focus',
      kids: Number.isFinite(b.minAge) ? b.minAge < 13 : true, social: b.social || 'block', lang: cfg.lang || 'en',
    };
  };
  const writeState = (cfg) => { const s = focusState(cfg); if (s) api.storage.local.set({ [K.focus]: s }).catch(() => {}); };
  ctx.onConfig(writeState);

  const whenReady = async () => {
    for (let i = 0; i < 50 && !ctx.ready; i++) await new Promise((r) => setTimeout(r, 20));
    try { await ctx.ready; } catch { /* core failed; answer with defaults */ }
  };

  api.runtime.onMessage.addListener((msg, sender, sendResponse) => {
    if (!msg || typeof msg.type !== 'string' || !msg.type.startsWith('kg:focus:')) return false;
    if (sender.id !== api.runtime.id) return false;
    (async () => {
      await whenReady();
      switch (msg.type) {
        case 'kg:focus:videos': return videosFor(msg);
        case 'kg:focus:channels': {
          await loadData();
          const band = String(msg.ageBand || (ctx.getConfig() || {}).ageBand || '');
          return { channels: (channels || []).filter((c) => Array.isArray(c.bands) && c.bands.includes(band) && CH_RE.test(c.id)).map((c) => ({ id: c.id, handle: c.handle || '', name: c.name || '' })) };
        }
        case 'kg:focus:i18n': return { strings: await strings(String(msg.lang || (ctx.getConfig() || {}).lang || 'en')) };
        case 'kg:focus:state': return focusState(ctx.getConfig()) || { focusMode: true, ageBand: '9-12', youtubeMode: 'focus', kids: true, social: 'block', lang: 'en' };
        default: return { error: 'unknown' };
      }
    })().then(sendResponse, (e) => sendResponse({ error: String((e && e.message) || e) }));
    return true;
  });

  // Keep the current band's feeds warm: every 6 hours, and once a little after start.
  try {
    api.alarms.get(ALARM).then((a) => { if (!a) api.alarms.create(ALARM, { periodInMinutes: 360, delayInMinutes: 1 }); }).catch(() => {});
    api.alarms.onAlarm.addListener((a) => {
      if (a.name !== ALARM) return;
      whenReady().then(() => { const c = ctx.getConfig(); if (c && c.focusMode !== false) return videosFor({ ageBand: c.ageBand, lang: c.lang, limit: 12 }); }).catch(() => {});
    });
  } catch { /* alarms unavailable */ }
  whenReady().then(() => writeState(ctx.getConfig())).catch(() => {});
}
