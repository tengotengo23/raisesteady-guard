// Toolbar popup: time left, what is on, one honest line, the parent link.
import { APP_NAME } from '../ui/brand.js';
import { loadI18n, applyI18n, fmtMinutes } from '../ui/i18n.js';
import { api, getStatus, el, setMeter } from '../ui/page.js';

const $ = (id) => document.getElementById(id);

async function main() {
  const t = await loadI18n('core');
  applyI18n(t);
  document.title = APP_NAME;
  $('name').textContent = APP_NAME;
  const s = await getStatus();
  if (!s) return;
  const extra = s.extraMin || 0;
  for (const [k, left, min] of [['browse', s.browseLeftSec, s.limits.browseMin], ['video', s.videoLeftSec, s.limits.videoMin]]) {
    $(`${k}Left`).textContent = left == null ? t('noLimit') : left <= 0 ? t('usedUp') : t('leftFmt', { t: fmtMinutes(t, left) });
    setMeter($(`${k}Meter`), left, min == null ? null : min + extra);
  }
  const row = (label, on) => el('li', {}, el('span', { text: label }), el('span', { class: on ? 'pp-on' : 'pp-off', text: t(on ? 'on' : 'off') }));
  $('list').replaceChildren(
    row(t('f_safesearch'), true),
    row(t('f_filter'), true),
    row(t('f_focus'), s.config.focusMode),
    row(t('f_ads'), s.config.adblock),
    el('li', {}, el('span', { text: s.bedtime ? t('f_bedtime', s.bedtime) : t('f_bedtimeOff') })),
  );
  $('setup').hidden = !(s.standalone && !s.setupDone);
  // Pause and homework time (P2-160), and the honest line about searches while they are recorded.
  const now = s.paused ? t('r_paused_t', { until: s.pausedUntil || '' }) : s.homework ? t('r_homework_t', { end: s.homeworkEnd || '' }) : '';
  $('now').textContent = now;
  $('now').hidden = !now;
  $('honestSearches').hidden = !s.searchesOn;
  const link = $('parent');
  link.href = s.parentUrl;
  if (s.standalone) {
    link.removeAttribute('target');
    link.addEventListener('click', (e) => { e.preventDefault(); api.runtime.openOptionsPage(); window.close(); });
  }
}

main();
