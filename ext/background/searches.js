// Search words from result pages (P2-160, F4). Pure (unit-tested). Only the address is read, never the page.
// Google, Bing, DuckDuckGo, Yahoo, Ecosia, Yandex and YouTube search; trimmed, lower case, at most 100 characters.

export const MAX_SEARCH_LEN = 100;

const ENGINES = [
  // [host test, path test, query parameter]
  [(h) => /(^|\.)google\.[a-z.]{2,6}$/.test(h) && !/^(mail|docs|drive|maps|news|play|translate)\./.test(h), (p) => p === '/search', 'q'],
  [(h) => /(^|\.)bing\.com$/.test(h), (p) => p === '/search' || p === '/videos/search' || p === '/images/search', 'q'],
  [(h) => /(^|\.)duckduckgo\.com$/.test(h), (p) => p === '/' || p === '/html' || p === '/html/' || p === '/lite' || p === '/lite/', 'q'],
  [(h) => /(^|\.)search\.yahoo\.com$/.test(h), (p) => p.startsWith('/search'), 'p'],
  [(h) => /(^|\.)ecosia\.org$/.test(h), (p) => p === '/search' || p === '/videos' || p === '/images', 'q'],
  [(h) => /(^|\.)yandex\.(ru|com|by|kz|uz|com\.tr|com\.am|com\.ge)$/.test(h) || /(^|\.)ya\.ru$/.test(h), (p) => p === '/search' || p === '/search/' || p === '/video/search' || p === '/images/search', 'text'],
  [(h) => /(^|\.)youtube\.com$/.test(h), (p) => p === '/results', 'search_query'],
];

/** Cleans a search: trimmed, lower case, spaces folded, at most 100 characters; '' when empty. */
export function cleanSearch(q) {
  let s = String(q ?? '').toLowerCase().replace(/[\u0000-\u001f\u007f]+/g, ' ').replace(/\s+/g, ' ').trim();
  const chars = [...s];
  if (chars.length > MAX_SEARCH_LEN) s = chars.slice(0, MAX_SEARCH_LEN).join('').trim();
  return s;
}

/** The search words of a result page's address, or null. */
export function searchQuery(url) {
  let u;
  try { u = new URL(url); } catch { return null; }
  if (u.protocol !== 'https:' && u.protocol !== 'http:') return null;
  const host = u.hostname.toLowerCase().replace(/^www\./, '').replace(/^m\./, '');
  for (const [hostOk, pathOk, param] of ENGINES) {
    if (!hostOk(host) || !pathOk(u.pathname)) continue;
    const q = cleanSearch(u.searchParams.get(param));
    return q || null;
  }
  return null;
}
