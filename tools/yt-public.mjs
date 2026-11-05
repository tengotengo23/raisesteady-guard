// RaiseSteady Guard (P2-175): reads YouTube's public pages at build time, with no API key and no outside code.
// A channel's Videos tab and a search result page both carry the page's own data as JSON (ytInitialData);
// only these fields are kept: id, title, channel, channel id, length, views, the "ago" text, a description line.
// Used by tools/update-videos.mjs (the built-in videos) and tools/update-values-videos.mjs (the values pool).
// Reuse search (P2-175): yt-dlp (Unlicense) did this before, but running a downloaded binary was refused by the
// permission check on 5 Nov; youtubei.js (MIT) would be outside code too. This reader is short and reads data only.

export const HEADERS = {
  'user-agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36',
  'accept-language': 'en-GB,en;q=0.8',
  cookie: 'CONSENT=YES+1; SOCS=CAI',
};
const ID_RE = /^[\w-]{11}$/;
const CH_RE = /^UC[\w-]{22}$/;
const clean = (s, n = 200) => String(s ?? '').replace(/[\u0000-\u001f\u007f]+/g, ' ').replace(/\s+/g, ' ').trim().slice(0, n);
const runs = (x) => (x && (x.simpleText != null ? x.simpleText : Array.isArray(x.runs) ? x.runs.map((r) => r.text || '').join('') : x.content)) || '';

/** The ytInitialData object of a page, or null. Pure. */
export function initialData(html) {
  const m = /var ytInitialData = (\{[\s\S]*?\});<\/script>/.exec(String(html || '')) || /window\["ytInitialData"\] = (\{[\s\S]*?\});/.exec(String(html || ''));
  if (!m) return null;
  try { return JSON.parse(m[1]); } catch { return null; }
}

/** "57:21" or "1:02:03" -> seconds; else 0. Pure. */
export function lengthSec(text) {
  const p = String(text || '').trim().split(':').map((x) => Number(x));
  if (p.length < 2 || p.length > 3 || p.some((x) => !Number.isFinite(x))) return 0;
  return p.reduce((s, x) => s * 60 + x, 0);
}

/** "230,769 views", "6.1k", "1.2M views", "No views" -> a number (0 when unknown). Pure. */
export function viewsOf(text) {
  const s = String(text || '').toLowerCase().replace(/,/g, '').trim();
  const m = /([\d.]+)\s*([km])?/.exec(s);
  if (!m) return 0;
  const n = Number(m[1]) * (m[2] === 'k' ? 1e3 : m[2] === 'm' ? 1e6 : 1);
  return Number.isFinite(n) ? Math.round(n) : 0;
}

/** "3 days ago", "1 hr ago", "6 mo ago", "6y ago", "3w ago", "Streamed 2 years ago" (English page) -> 'YYYY-MM-DD'
 *  (approximate), else ''. Pure. */
export function agoToDate(text, now = Date.now()) {
  const m = /(\d+)\s*(seconds?|secs?|s|minutes?|mins?|hours?|hrs?|h|days?|d|weeks?|wks?|w|months?|mos?|years?|yrs?|y|m)\s+ago\b/i.exec(String(text || ''));
  if (!m) return '';
  const u = m[2].toLowerCase();
  const sec = /^s/.test(u) ? 1 : /^mo/.test(u) ? 2629800 : /^m/.test(u) ? 60 : /^h/.test(u) ? 3600 : /^d/.test(u) ? 86400 : /^w/.test(u) ? 604800 : 31557600;
  return new Date(now - Number(m[1]) * sec * 1000).toISOString().slice(0, 10);
}

/** Every video on a page (search results: videoRenderer; channel tabs: lockupViewModel). Shorts, lives,
 *  playlists and channels are left out. channelName/channelId: for a channel page, whose videos these are. Pure. */
export function videosFrom(data, { channelName = '', channelId = '' } = {}) {
  const out = [], seen = new Set();
  const add = (v) => { if (v && ID_RE.test(v.id) && v.title && !seen.has(v.id)) { seen.add(v.id); out.push(v); } };
  const walk = (o, depth) => {
    if (!o || typeof o !== 'object' || depth > 40) return;
    if (Array.isArray(o)) { for (const x of o) walk(x, depth + 1); return; }
    const vr = o.videoRenderer;
    if (vr && typeof vr === 'object') {
      const owner = (vr.ownerText || vr.longBylineText || {}).runs || [];
      const be = owner[0] && owner[0].navigationEndpoint && owner[0].navigationEndpoint.browseEndpoint;
      const live = (vr.badges || []).some((b) => /LIVE/.test(JSON.stringify(b))) || !vr.lengthText;
      const snip = ((vr.detailedMetadataSnippets || [])[0] || {}).snippetText || vr.descriptionSnippet;
      if (!live) add({ id: vr.videoId, title: clean(runs(vr.title)), channel: clean(owner[0] && owner[0].text, 100), channelId: be && CH_RE.test(be.browseId) ? be.browseId : '',
        lengthSec: lengthSec(runs(vr.lengthText)), views: viewsOf(runs(vr.viewCountText)), ago: clean(runs(vr.publishedTimeText), 40), desc: clean(runs(snip), 300) });
      return;
    }
    const lv = o.lockupViewModel;
    if (lv && typeof lv === 'object' && lv.contentType === 'LOCKUP_CONTENT_TYPE_VIDEO') {
      const md = (lv.metadata && lv.metadata.lockupMetadataViewModel) || {};
      const parts = [];
      for (const row of (((md.metadata || {}).contentMetadataViewModel || {}).metadataRows || [])) for (const p of row.metadataParts || []) parts.push(runs(p.text));
      let badge = '';
      const findBadge = (x, d) => { if (badge || !x || typeof x !== 'object' || d > 12) return; if (x.thumbnailBadgeViewModel && x.thumbnailBadgeViewModel.text) { badge = x.thumbnailBadgeViewModel.text; return; } for (const v of Object.values(x)) findBadge(v, d + 1); };
      findBadge(lv.contentImage, 0);
      const len = lengthSec(badge);
      if (len > 0) add({ id: lv.contentId, title: clean(runs(md.title)), channel: clean(channelName, 100), channelId, lengthSec: len,
        views: viewsOf(parts.find((x) => /^[\d.,]+\s*[km]?(\s*views?)?$/i.test(String(x).trim())) || ''), ago: clean(parts.find((x) => /ago/i.test(x)) || '', 40), desc: '' });
      return;
    }
    for (const v of Object.values(o)) walk(v, depth + 1);
  };
  walk(data, 0);
  return out;
}

async function page(url) {
  const r = await fetch(url, { headers: HEADERS, redirect: 'follow', signal: AbortSignal.timeout(30000) });
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  const data = initialData(await r.text());
  if (!data) throw new Error('no page data');
  return data;
}

/** The newest videos of a channel's Videos tab (English page, so the "ago" texts are English). */
export async function channelVideos(id, name = '') {
  if (!CH_RE.test(id)) throw new Error('bad channel id');
  const data = await page(`https://www.youtube.com/channel/${id}/videos?hl=en&persist_hl=1`);
  const meta = (data.metadata || {}).channelMetadataRenderer || {};
  return videosFrom(data, { channelName: name || meta.title || '', channelId: id });
}

/** One page of search results, videos only (sp=EgIQAQ%3D%3D). */
export async function search(q, { hl = 'en' } = {}) {
  const data = await page(`https://www.youtube.com/results?search_query=${encodeURIComponent(q)}&sp=EgIQAQ%253D%253D&hl=${encodeURIComponent(hl)}`);
  return videosFrom(data);
}
