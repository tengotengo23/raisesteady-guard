// Helpers for the extension pages (block page, new tab, popup, options).

import { parseLearning, isLearningUrl } from '../background/domains.js';

export const api = globalThis.browser ?? globalThis.chrome;

/** Ask the background; resolves to null when it does not answer. */
export async function ask(type, data = {}) {
  try { return await api.runtime.sendMessage({ type: `kg:core:${type}`, ...data }); } catch { return null; }
}

/** Status from the background; retries briefly while the service worker starts. */
export async function getStatus() {
  for (let i = 0; i < 6; i++) {
    const s = await ask('status');
    if (s && s.config) return s;
    await new Promise((r) => setTimeout(r, 150 * (i + 1)));
  }
  return null;
}

/** Learning bookmarks for the band: [{ title, url, desc, cost, folder }]. learningOnly: only sites that stay open when time is up. */
export async function bandBookmarks(band, lang, limit = 12, { learningOnly = false } = {}) {
  try {
    const r = await fetch(api.runtime.getURL('data/bookmarks.json'));
    if (!r.ok) return [];
    const data = await r.json();
    let learn = null;
    if (learningOnly) {
      const lr = await fetch(api.runtime.getURL('data/learning-domains.json'));
      learn = parseLearning(lr.ok ? (await lr.json()).domains : []);
    }
    const out = [];
    for (const f of data.folders || []) {
      for (const it of f.items || []) {
        if (Array.isArray(it.bands) && !it.bands.includes(band)) continue;
        if (!/^https:\/\//.test(it.url || '')) continue;
        if (learn && !isLearningUrl(it.url, learn)) continue;
        out.push({ title: it.title, url: it.url, desc: (it.desc && (it.desc[lang] || it.desc.en)) || '', cost: it.cost, folder: (f.title && (f.title[lang] || f.title.en)) || '' });
      }
    }
    // One from each folder first, so the list is varied.
    const seen = new Set(), first = [], rest = [];
    for (const b of out) (seen.has(b.folder) ? rest : (seen.add(b.folder), first)).push(b);
    return [...first, ...rest].slice(0, limit);
  } catch { return []; }
}

export function el(tag, attrs = {}, ...kids) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === 'class') n.className = v;
    else if (k === 'text') n.textContent = v;
    else if (k.startsWith('on') && typeof v === 'function') n.addEventListener(k.slice(2), v);
    else n.setAttribute(k, v === true ? '' : v);
  }
  for (const k of kids.flat()) if (k != null) n.append(k);
  return n;
}

export function bookmarkTiles(list, t) {
  return el('ul', { class: 'kg-tiles' }, list.map((b) => el('li', {},
    el('a', { class: 'kg-tile', href: b.url, rel: 'noopener' },
      el('strong', { text: b.title }),
      b.desc ? el('span', { text: b.desc }) : null,
      b.cost === 'free-with-paid-extras' ? el('em', { class: 'kg-tag', text: t('freeExtras') }) : null))));
}

/** Meter fill: % used, class by what is left. */
export function setMeter(meter, leftSec, limitMin) {
  if (leftSec == null || !limitMin) { meter.hidden = true; return; }
  const total = limitMin * 60;
  const used = Math.min(1, Math.max(0, 1 - leftSec / total));
  meter.hidden = false;
  meter.style.setProperty('--kg-fill', `${Math.round((1 - used) * 100)}%`);
  meter.classList.toggle('kg-low', leftSec > 0 && leftSec <= 600);
  meter.classList.toggle('kg-out', leftSec <= 0);
}
