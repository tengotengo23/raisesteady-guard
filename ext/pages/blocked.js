// Block page: calm words for each reason, what to do next, and "Ask a parent" (PIN, then allow or add time).
import { APP_NAME } from '../ui/brand.js';
import { loadI18n, applyI18n } from '../ui/i18n.js';
import { ask, getStatus, bandBookmarks, bookmarkTiles, el } from '../ui/page.js';

const REASONS = ['adult', 'gambling', 'bypass', 'social', 'content', 'parent', 'time', 'video', 'bedtime', 'shorts', 'curated', 'paused', 'homework', 'topic'];
const ALLOWABLE = ['adult', 'gambling', 'bypass', 'social', 'content', 'parent', 'homework', 'topic'];
const TIMED = ['time', 'video'];

const ART = {
  leaf: '<svg viewBox="0 0 96 96"><circle cx="48" cy="48" r="46" fill="var(--kg-mint-light)"/><path d="M30 64c0-20 14-34 36-36-1 22-15 36-34 36z" fill="var(--kg-mint)"/><path d="M30 66c8-10 16-18 28-26" stroke="var(--kg-navy)" stroke-width="3" stroke-linecap="round" fill="none"/></svg>',
  sun: '<svg viewBox="0 0 96 96"><circle cx="48" cy="48" r="46" fill="var(--kg-soft)"/><circle cx="48" cy="48" r="18" fill="var(--kg-yellow)"/><g stroke="var(--kg-yellow)" stroke-width="5" stroke-linecap="round"><path d="M48 14v8M48 74v8M14 48h8M74 48h8M24 24l6 6M66 66l6 6M24 72l6-6M66 30l6-6"/></g></svg>',
  play: '<svg viewBox="0 0 96 96"><circle cx="48" cy="48" r="46" fill="var(--kg-soft)"/><rect x="22" y="30" width="52" height="36" rx="10" fill="var(--kg-coral)"/><path d="M43 39l14 9-14 9z" fill="#fff"/></svg>',
  moon: '<svg viewBox="0 0 96 96"><circle cx="48" cy="48" r="46" fill="var(--kg-navy)"/><path d="M58 24a26 26 0 1 0 14 40 22 22 0 0 1-14-40z" fill="var(--kg-yellow)"/><circle cx="30" cy="30" r="2" fill="#fff"/><circle cx="70" cy="26" r="1.6" fill="#fff"/></svg>',
};

const params = new URLSearchParams(location.search);
const reason = REASONS.includes(params.get('reason')) ? params.get('reason') : 'content';
const $ = (id) => document.getElementById(id);

async function main() {
  const t = await loadI18n('core');
  applyI18n(t);
  const svg = reason === 'bedtime' ? ART.moon : reason === 'time' || reason === 'paused' ? ART.sun : reason === 'video' || reason === 'shorts' || reason === 'curated' ? ART.play : ART.leaf;
  const doc = new DOMParser().parseFromString(svg.replace('<svg ', '<svg xmlns="http://www.w3.org/2000/svg" '), 'image/svg+xml');
  $('art').replaceChildren(document.importNode(doc.documentElement, true));

  const [status, att] = await Promise.all([getStatus(), ask('attempted')]);
  const site = (att && att.host) || sanitizeHost(params.get('site'));
  const targetUrl = att && att.url;
  const end = reason === 'homework' ? (status && status.homeworkEnd) || '' : status && status.bedtime ? status.bedtime.end : '';
  const until = (status && status.pausedUntil) || '';

  $('title').textContent = t(`r_${reason}_t`, { end, until });
  document.title = `${$('title').textContent} · ${APP_NAME}`;
  $('body').textContent = t(`r_${reason}_b`, { end, until });
  // Topic page (P2-161): under the words, a way to talk to someone. Helplines are never blocked.
  if (reason === 'topic') $('help').hidden = false;
  // At night and in a pause, the same way to talk to someone, under the words.
  if (reason === 'bedtime' || reason === 'paused') { $('help').prepend(`${t('r_topic_b')} `); $('help').hidden = false; }
  if (site) { $('site').hidden = false; $('site').replaceChildren(el('span', { class: 'bp-site', text: t('triedSite', { site }) })); }

  if (reason === 'bedtime') {
    $('bedTip').hidden = false;
  } else if (status) {
    // When time is up only learning sites open, so show only those.
    const list = reason === 'paused' ? [] : await bandBookmarks(status.config.ageBand, t.lang, 4, { learningOnly: reason === 'time' || reason === 'homework' });
    if (list.length) { $('tiles').replaceChildren(bookmarkTiles(list, t)); $('next').hidden = false; }
  }

  ask('blocked-shown', { reason, site });
  setupAsk(t, status, site, targetUrl);
}

function sanitizeHost(h) {
  const v = String(h || '').toLowerCase();
  return /^[a-z0-9.-]{1,253}$/.test(v) && v.includes('.') ? v : '';
}

function setupAsk(t, status, site, targetUrl) {
  if (reason === 'bedtime') { $('ask').hidden = false; $('askBtn').hidden = true; $('bedAsk').hidden = false; return; }
  const paused = reason === 'paused' && !!(status && !status.standalone);
  const canAllow = ALLOWABLE.includes(reason) && !!site;
  if (!canAllow && !TIMED.includes(reason) && !paused) return;
  $('ask').hidden = false;
  if (!paused && status && status.canRequest) setupRequest(t, canAllow ? { kind: 'site', domain: site } : { kind: 'time' });
  const btn = $('askBtn'), panel = $('askPanel');
  if (status && status.standalone && !status.hasPin) {
    btn.addEventListener('click', () => { showMsg('pinMsg', t('noPinYet')); btn.setAttribute('aria-expanded', 'true'); panel.hidden = false; $('pinForm').hidden = true; });
    return;
  }
  btn.addEventListener('click', () => {
    const open = panel.hidden;
    panel.hidden = !open;
    btn.setAttribute('aria-expanded', String(open));
    if (open) $('pin').focus();
  });
  let pin = '';
  $('pinForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    hide('pinMsg');
    const v = $('pin').value.trim();
    if (!/^\d{4,8}$/.test(v)) return showMsg('pinMsg', t('pinFormat'));
    const r = await ask('pin', { pin: v });
    if (!r || !r.ok) return showMsg('pinMsg', t(r && r.offline ? 'pinOffline' : r && r.locked ? 'pinLocked' : r && r.noPin ? 'noPinYet' : 'pinWrong'));
    pin = v;
    $('pinForm').hidden = true;
    const box = $('grants');
    box.replaceChildren();
    if (paused) box.append(el('button', { class: 'kg-btn', type: 'button', text: t('endPause'), onclick: () => doGrant({ kind: 'resume' }) }));
    if (canAllow) box.append(el('button', { class: 'kg-btn', type: 'button', text: t('allowSite', { site }), onclick: () => doGrant({ kind: 'allow', domain: site }) }));
    if (TIMED.includes(reason)) for (const n of [15, 30, 60]) box.append(el('button', { class: 'kg-btn kg-sun', type: 'button', text: t('addMin', { n }), onclick: () => doGrant({ kind: 'extra', minutes: n }) }));
    $('grantBox').hidden = false;
    const first = box.querySelector('button');
    if (first) first.focus();
  });

  async function doGrant(g) {
    for (const b of $('grants').querySelectorAll('button')) b.disabled = true;
    const r = await ask('grant', { pin, ...g });
    if (!r || !r.ok) {
      for (const b of $('grants').querySelectorAll('button')) b.disabled = false;
      return showMsg('grantMsg', t(r && r.offline ? 'pinOffline' : r && r.locked ? 'pinLocked' : 'pinWrong'), true);
    }
    showMsg('grantMsg', g.kind === 'extra' ? t('grantedTime', { n: g.minutes }) : g.kind === 'resume' ? t('pauseEnded') : t('granted'));
    if (targetUrl) setTimeout(() => { location.href = targetUrl; }, 900);
  }
}

// "Ask a parent" (P2-160): records a request for the parent page; no PIN. The answer shows under the button.
function setupRequest(t, what) {
  const form = $('req'), btn = $('reqBtn');
  form.hidden = false;
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    btn.disabled = true;
    const r = await ask('request', { ...what, note: $('reqNote').value.slice(0, 140) });
    if (r && r.ok) {
      showMsg('reqMsg', t('reqAsked'));
      $('reqNote').disabled = true;
      return;
    }
    btn.disabled = false;
    showMsg('reqMsg', t('reqFailed'), true);
  });
}

function showMsg(id, text, bad) {
  const p = $(id);
  p.textContent = text;
  p.hidden = false;
  if (id === 'grantMsg' || id === 'reqMsg') { p.classList.toggle('kg-bad', !!bad); p.classList.toggle('kg-good', !bad); }
}
function hide(id) { $(id).hidden = true; }

main();
