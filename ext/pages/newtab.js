// New tab: greeting, time left, a safe search box, the band's bookmarks, then curated videos.
import { loadI18n, applyI18n, fmtMinutes } from '../ui/i18n.js';
import { getStatus, bandBookmarks, bookmarkTiles, setMeter } from '../ui/page.js';

const $ = (id) => document.getElementById(id);

function greeting(t) {
  const h = new Date().getHours();
  return t(h < 5 ? 'greetNight' : h < 12 ? 'greetMorning' : h < 18 ? 'greetDay' : h < 23 ? 'greetEvening' : 'greetNight');
}

function showTime(t, s) {
  const extra = s.extraMin || 0;
  const rows = [['browse', s.browseLeftSec, s.limits.browseMin], ['video', s.videoLeftSec, s.limits.videoMin]];
  for (const [k, left, min] of rows) {
    $(`${k}Left`).textContent = left == null ? t('noLimit') : left <= 0 ? t('usedUp') : t('leftFmt', { t: fmtMinutes(t, left) });
    setMeter($(`${k}Meter`), left, min == null ? null : min + extra);
  }
}

async function main() {
  const t = await loadI18n('core');
  applyI18n(t);
  $('greet').textContent = greeting(t);
  const s = await getStatus();
  if (!s) return;
  showTime(t, s);
  setInterval(async () => { const n = await getStatus(); if (n) showTime(t, n); }, 30000);

  const list = await bandBookmarks(s.config.ageBand, t.lang, 12);
  if (list.length) { $('tiles').replaceChildren(bookmarkTiles(list, t)); $('marks').hidden = false; }

  // The chosen videos come from YouTube, so they follow the YouTube focus switch:
  // with focus mode off the new tab page loads nothing from YouTube.
  if (s.config.focusMode !== false) try {
    const mod = await import('../ui/video-grid.js');
    if (typeof mod.renderVideoGrid === 'function') {
      $('vids').hidden = false;
      await mod.renderVideoGrid($('videoGrid'), { ageBand: s.config.ageBand, lang: t.lang, limit: 8 });
      if (!$('videoGrid').childElementCount) $('vids').hidden = true;
    }
  } catch {
    $('vids').hidden = true; // videos are optional; the page works without them
  }
}

main();
