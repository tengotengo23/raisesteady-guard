// Curated video grid for extension pages: the new tab page calls renderVideoGrid by dynamic import.
// Videos come from background/videos.js (kg:focus:videos); thumbnails from i.ytimg.com.
// content/focus-youtube.js draws the same cards on youtube.com with its own copy of this markup
// (content scripts cannot import modules).
import { loadI18n } from './i18n.js';

const api = globalThis.browser ?? globalThis.chrome;
const ID_RE = /^[\w-]{11}$/;

const CSS = `
.kgv-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 13.5rem), 1fr)); gap: 16px; list-style: none; margin: 0; padding: 0; }
.kgv-grid > li { min-width: 0; }
.kgv-card { display: flex; flex-direction: column; gap: 6px; height: 100%; min-height: var(--kg-tap, 44px); padding: 0 0 10px; border-radius: var(--kg-radius-s, 12px);
  color: var(--kg-text); text-decoration: none; background: var(--kg-surface); border: 1px solid var(--kg-line); overflow: hidden;
  transition: transform var(--kg-time, 220ms) var(--kg-ease, ease), border-color var(--kg-time, 220ms) var(--kg-ease, ease); }
.kgv-card:hover { transform: translateY(-2px); border-color: var(--kg-accent); }
.kgv-card:focus-visible { outline: 3px solid var(--kg-focus); outline-offset: 2px; }
.kgv-thumb { display: block; width: 100%; aspect-ratio: 16 / 9; object-fit: cover; background: var(--kg-soft); }
.kgv-title { padding: 0 12px; font-weight: 650; line-height: 1.35; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
.kgv-ch { padding: 0 12px; font-size: var(--kg-size-s, 0.9375rem); color: var(--kg-text-2); overflow-wrap: anywhere; }
.kgv-state { margin: 0; padding: 14px 16px; border-radius: var(--kg-radius-s, 12px); background: var(--kg-soft); color: var(--kg-text); }
.kgv-busy { min-height: 6rem; border-radius: var(--kg-radius-s, 12px); background: var(--kg-soft); opacity: .6; }
@media (prefers-reduced-motion: reduce) {
  .kgv-card { transition: opacity var(--kg-time, 220ms) var(--kg-ease, ease), border-color var(--kg-time, 220ms) var(--kg-ease, ease); }
  .kgv-card:hover { transform: none; opacity: .92; }
}`;

function addStyle() {
  if (document.getElementById('kgv-style')) return;
  const s = document.createElement('style');
  s.id = 'kgv-style';
  s.textContent = CSS;
  document.head.append(s);
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

/** One card: a single link (thumbnail, title, channel). */
export function videoCard(v, t) {
  const a = el('a', 'kgv-card');
  a.href = `https://www.youtube.com/watch?v=${v.id}`;
  a.setAttribute('aria-label', t('cardLabel', { title: v.title, channel: v.channel }));
  const img = el('img', 'kgv-thumb');
  img.src = `https://i.ytimg.com/vi/${v.id}/mqdefault.jpg`;
  img.alt = '';
  img.loading = 'lazy';
  img.decoding = 'async';
  img.width = 320; img.height = 180;
  img.addEventListener('error', () => { img.removeAttribute('src'); }, { once: true });
  a.append(img, el('span', 'kgv-title', v.title), el('span', 'kgv-ch', v.channel));
  return a;
}

/**
 * Render curated videos for the band into `container`. Always leaves something calm in it:
 * the grid, or one line saying the videos are on their way / need the internet.
 */
export async function renderVideoGrid(container, { ageBand, lang, limit = 8 } = {}) {
  if (!container) return;
  addStyle();
  const t = await loadI18n('focus', lang);
  container.setAttribute('aria-busy', 'true');
  container.replaceChildren(el('div', 'kgv-busy'));
  let res = null;
  try { res = await api.runtime.sendMessage({ type: 'kg:focus:videos', ageBand, lang: t.lang, limit }); } catch { res = null; }
  container.removeAttribute('aria-busy');
  const videos = (res && Array.isArray(res.videos) ? res.videos : []).filter((v) => v && ID_RE.test(v.id));
  if (!videos.length) {
    const offline = (globalThis.navigator && navigator.onLine === false) || (res && res.offline);
    container.replaceChildren(el('p', 'kgv-state', t(offline ? 'gridOffline' : 'gridEmpty')));
    return;
  }
  const ul = el('ul', 'kgv-grid');
  ul.setAttribute('aria-label', t('gridLabel'));
  for (const v of videos) { const li = el('li'); li.append(videoCard(v, t)); ul.append(li); }
  container.replaceChildren(ul);
}
