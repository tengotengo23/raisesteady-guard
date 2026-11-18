// News from RaiseSteady (P2-252): pure helpers for the options page in standalone mode. The feed is read only
// when a parent has unlocked the options page and turned news on; never on a child screen, never on a timer.
// Rules (the same as the Windows app's agent/internal/news): v must be 1, at most 5 items, id [0-9a-z-]{1,40},
// dates YYYY-MM-DD, title at most 80 and body at most 600 characters of plain text (\n allowed in the body),
// title.en and body.en required, link only https://raisesteady.com/… or https://github.com/tengotengo23/….

export const FEED_URL = 'https://raisesteady.com/guard/news.json';
export const TTL = 6 * 60 * 60 * 1000;
export const TIMEOUT = 3000;
export const MAX_BYTES = 32 * 1024;
export const MAX_ITEMS = 5;
export const MAX_TITLE = 80;
export const MAX_BODY = 600;
export const MAX_HIDDEN = 100;
export const RETRY = 10 * 60 * 1000; // after a failed read, as the agent waits
export const LANGS = ['en', 'ka', 'ru'];

const ID_RE = /^[0-9a-z-]{1,40}$/;
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
// C0, DEL, C1 and the two JavaScript line terminators that are not \n.
const CONTROL_RE = /[\u0000-\u001f\u007f-\u009f\u2028\u2029]/;

export function validId(id) { return typeof id === 'string' && ID_RE.test(id); }

export function validDate(s) {
  if (typeof s !== 'string' || !DATE_RE.test(s)) return false;
  const d = new Date(`${s}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === s;
}

function plain(s, max, lines) {
  if (typeof s !== 'string' || s.trim() === '' || [...s].length > max) return false;
  return !CONTROL_RE.test(lines ? s.replace(/\n/g, '') : s);
}

function texts(m, max, lines) {
  if (!m || typeof m !== 'object' || Array.isArray(m)) return null;
  const out = {};
  for (const l of LANGS) if (Object.hasOwn(m, l) && plain(m[l], max, lines)) out[l] = m[l];
  return out.en ? out : null;
}

/** Whether a link may be shown: https on raisesteady.com, or on github.com under /tengotengo23/. */
export function safeLink(link) {
  if (typeof link !== 'string' || link.length > 300 || /[\s\\"'<>]/.test(link)) return false;
  let host;
  if (link.startsWith('https://raisesteady.com/')) host = 'raisesteady.com';
  else if (link.startsWith('https://github.com/tengotengo23/')) host = 'github.com';
  else return false;
  // No dot segments (plain or %2e): the browser would resolve them out of the allowed path.
  if (link.split(/[?#]/)[0].includes('/.') || /%2e/i.test(link)) return false;
  let u;
  try { u = new URL(link); } catch { return false; }
  if (u.protocol !== 'https:' || u.hostname !== host || u.port !== '' || u.username !== '' || u.password !== '') return false;
  return host !== 'github.com' || u.pathname.startsWith('/tengotengo23/');
}

function item(raw) {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  if (!validId(raw.id) || !validDate(raw.date)) return null;
  const it = { id: raw.id, date: raw.date };
  if (raw.until !== undefined && raw.until !== null) {
    if (!validDate(raw.until)) return null;
    it.until = raw.until;
  }
  it.title = texts(raw.title, MAX_TITLE, false);
  it.body = texts(raw.body, MAX_BODY, true);
  if (!it.title || !it.body) return null;
  if (safeLink(raw.link)) it.link = raw.link; // a bad link is dropped, the note kept
  return it;
}

function byteLength(s) { return new TextEncoder().encode(s).length; }

/** The valid items of a feed (text), or null when the whole file is to be ignored. */
export function parseFeed(text) {
  if (typeof text !== 'string' || byteLength(text) > MAX_BYTES) return null;
  let f;
  try { f = JSON.parse(text); } catch { return null; }
  if (!f || typeof f !== 'object' || f.v !== 1) return null;
  const out = [];
  for (const raw of Array.isArray(f.items) ? f.items : []) {
    if (out.length === MAX_ITEMS) break;
    const it = item(raw);
    if (it) out.push(it);
  }
  return out;
}

/** The text in lang, or in English. */
export function pickText(m, lang) { return (m && m[lang]) || (m && m.en) || ''; }

/** The local day as YYYY-MM-DD. */
export function localToday(d = new Date()) {
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Items not hidden and before their until day. */
export function visibleItems(items, today, hidden = []) {
  const h = new Set(Array.isArray(hidden) ? hidden : []);
  return (items || []).filter((it) => !h.has(it.id) && !(it.until && today >= it.until));
}

/** Whether the cache {fetched, feed} needs a new read. */
export function isStale(cache, now = Date.now()) {
  const at = cache && Number(cache.fetched);
  return !at || now - at >= TTL || now < at;
}

/** The hidden list with id added once, the oldest dropped past MAX_HIDDEN; null for a bad id. */
export function hideId(hidden, id) {
  if (!validId(id)) return null;
  const list = (Array.isArray(hidden) ? hidden : []).filter(validId).filter((x) => x !== id);
  list.push(id);
  return list.slice(-MAX_HIDDEN);
}

/** The body as text, or null past MAX_BYTES (read in chunks: a reply without Content-Length is never read whole). */
async function readCapped(res) {
  if (!res.body || typeof res.body.getReader !== 'function') { const t = await res.text(); return new TextEncoder().encode(t).length > MAX_BYTES ? null : t; }
  const reader = res.body.getReader(), parts = []; let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.length;
    if (size > MAX_BYTES) { try { await reader.cancel(); } catch { /* gone */ } return null; }
    parts.push(value);
  }
  const all = new Uint8Array(size); let at = 0;
  for (const p of parts) { all.set(p, at); at += p.length; }
  return new TextDecoder('utf-8', { fatal: true }).decode(all);
}

/** Read the feed: no cookies, no referrer, no redirects, 3 s; the text, or null on any problem. */
export async function fetchFeed(fetchFn = globalThis.fetch) {
  try {
    const res = await fetchFn(FEED_URL, { credentials: 'omit', redirect: 'error', referrerPolicy: 'no-referrer', signal: AbortSignal.timeout(TIMEOUT) });
    if (!res || res.status !== 200) return null;
    const len = Number(res.headers && res.headers.get && res.headers.get('content-length'));
    if (len > MAX_BYTES) return null;
    const text = await readCapped(res);
    return text != null && parseFeed(text) ? text : null;
  } catch { return null; }
}
