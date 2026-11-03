// Small host helpers shared by the background modules. Pure functions.

/** Host of an http(s) URL without a leading "www.", lower case; null for other schemes. */
export function hostOf(url) {
  try {
    const u = new URL(url);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
    return u.hostname.toLowerCase().replace(/^www\./, '').replace(/\.$/, '') || null;
  } catch { return null; }
}

/** Normalise one parent-entered domain ("https://www.Example.org/x" -> "example.org"); null if invalid. */
export function cleanDomain(s) {
  let v = String(s ?? '').trim().toLowerCase();
  if (!v) return null;
  if (!/^[a-z]+:\/\//.test(v)) v = 'http://' + v;
  const h = hostOf(v);
  if (!h || !/^[a-z0-9.-]+$/.test(h) || !h.includes('.') || h.length > 253) return null;
  return h;
}

export function cleanDomains(list) {
  const out = new Set();
  for (const d of Array.isArray(list) ? list : []) { const c = cleanDomain(d); if (c) out.add(c); }
  return [...out];
}

/** True when host is one of the domains or a subdomain of one. */
export function inDomains(host, domains) {
  if (!host) return false;
  for (const d of domains || []) {
    if (host === d || host.endsWith('.' + d)) return true;
  }
  return false;
}

export function isLocalHost(host) {
  return host === '127.0.0.1' || host === 'localhost' || host === '[::1]';
}

/**
 * Hosts with many unrelated pages under one name (encyclopaedias, search, video, forums, code and
 * blog platforms that use paths per author). The page check never blocks these as a whole host.
 */
export const MIXED_HOSTS = [
  'wikipedia.org', 'wikimedia.org', 'wiktionary.org', 'wikibooks.org', 'wikihow.com', 'fandom.com',
  'youtube.com', 'youtu.be', 'bing.com', 'duckduckgo.com', 'yahoo.com', 'msn.com', 'ecosia.org', 'startpage.com',
  'archive.org', 'github.com', 'gitlab.com', 'reddit.com', 'medium.com', 'substack.com', 'quora.com',
  'stackexchange.com', 'stackoverflow.com', 'x.com', 'twitter.com', 'facebook.com', 'instagram.com', 'tiktok.com',
  'tumblr.com', 'pinterest.com', 'linkedin.com', 'vk.com', 'ok.ru', 'dzen.ru', 'mail.ru', 'livejournal.com', 'pikabu.ru',
  'telegram.org', 't.me', 'discord.com', 'imgur.com', 'vimeo.com', 'dailymotion.com', 'twitch.tv', 'rutube.ru',
  'amazon.com', 'ebay.com', 'microsoft.com', 'apple.com', 'translate.goog', 'myvideo.ge',
];
const MIXED_RE = /(^|\.)(google|yandex)\.[a-z]{2,3}(\.[a-z]{2})?$/;

export function isMixedHost(host) {
  return !!host && (inDomains(host, MIXED_HOSTS) || MIXED_RE.test(host));
}

/** Learning list: plain entries are hosts (with subdomains); "host/path" entries cover that path only. */
export function parseLearning(list) {
  const hosts = new Set(), paths = [];
  for (const raw of Array.isArray(list) ? list : []) {
    const v = String(raw || '').trim().toLowerCase();
    const i = v.indexOf('/');
    if (i < 0) { const h = cleanDomain(v); if (h) hosts.add(h); continue; }
    const h = cleanDomain(v.slice(0, i));
    const p = v.slice(i).replace(/\/+$/, '');
    if (h && p.length > 1 && /^\/[a-z0-9._~\/-]+$/.test(p)) paths.push({ host: h, path: p });
  }
  return { hosts: [...hosts], paths };
}

function pathMatch(u, paths) {
  const host = u.hostname.toLowerCase().replace(/^www\./, '');
  const path = u.pathname.toLowerCase();
  return paths.some((p) => (host === p.host || host.endsWith('.' + p.host)) && (path === p.path || path.startsWith(p.path + '/')));
}

/** True when the URL is on a learning host or under a learning path. */
export function isLearningUrl(url, L) {
  if (!L) return false;
  let u;
  try { u = new URL(url); } catch { return false; }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return false;
  return inDomains(hostOf(url), L.hosts) || pathMatch(u, L.paths || []);
}

/** "https://en.wikipedia.org/wiki/X?y" -> "https://en.wikipedia.org/wiki/X" (no query, no hash); null if not http(s). */
export function urlKey(url) {
  try {
    const u = new URL(url);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
    return `${u.protocol}//${u.host.toLowerCase()}${u.pathname}`;
  } catch { return null; }
}
