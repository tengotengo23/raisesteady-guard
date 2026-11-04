// Options: standalone setup (age, PIN, limits, bedtime, switches, lists). In managed mode only a note and the parent link.
import { loadI18n, applyI18n, fmtMinutes } from '../ui/i18n.js';
import { ask, getStatus, el } from '../ui/page.js';

const $ = (id) => document.getElementById(id);
let t, status, pin = '', settings = null, hasPin = false;

async function main() {
  t = await loadI18n('core');
  applyI18n(t);
  status = await getStatus();
  if (!status) return;
  if (!status.standalone) {
    document.title = t('optTitle');
    $('title').textContent = t('optTitle');
    $('managedNote').textContent = t('managedNote');
    $('parentLink').href = status.parentUrl;
    $('managed').hidden = false;
    return;
  }
  hasPin = status.hasPin;
  const heading = hasPin ? t('optTitle') : t('setupTitle');
  document.title = heading;
  $('title').textContent = heading;
  if (hasPin) {
    $('lock').hidden = false;
    $('lockPin').focus();
    $('lockForm').addEventListener('submit', async (e) => {
      e.preventDefault();
      const v = $('lockPin').value.trim();
      const r = await ask('settings-get', { pin: v });
      if (!r || !r.ok) { $('lockMsg').textContent = t(r && r.locked ? 'pinLocked' : /^\d{4,8}$/.test(v) ? 'pinWrong' : 'pinFormat'); $('lockMsg').hidden = false; return; }
      pin = v;
      $('lock').hidden = true;
      showForm(r.settings);
    });
  } else {
    const r = await ask('settings-get', {});
    showForm((r && r.settings) || {});
  }
}

function bandOf(id) { return status.bands.find((b) => b.id === id) || status.bands.find((b) => b.id === status.config.ageBand) || status.bands[0]; }

function fillFor(bandId, s) {
  const b = bandOf(bandId);
  const lim = (s && s.limits) || {};
  const set = (k) => {
    const v = lim[k] !== undefined ? lim[k] : b[k];
    $(`${k}`).value = v == null ? '' : v;
    $(`${k.replace('Min', 'None')}`).checked = v == null;
    $(`${k}`).disabled = v == null;
  };
  set('browseMin'); set('videoMin');
  const bt = s && s.bedtime !== undefined ? s.bedtime : b.bedtime;
  $('bedNone').checked = !bt;
  $('bedStart').value = bt ? bt.start.padStart(5, '0') : (b.bedtime ? b.bedtime.start : '20:00');
  $('bedEnd').value = bt ? bt.end.padStart(5, '0') : (b.bedtime ? b.bedtime.end : '07:00');
  $('bedStart').disabled = $('bedEnd').disabled = !bt;
  $('limitsHint').textContent = t('limitsHint', { b: b.browseMin, v: b.videoMin });
}

function showForm(s) {
  settings = s || {};
  $('intro').textContent = hasPin ? '' : t('setupIntro');
  $('intro').hidden = hasPin;
  $('pinKeep').hidden = !hasPin;
  const r = status.report || { blocked: 0 };
  if (hasPin) {
    $('today').textContent = `${t('todayTitle')}: ${t('todayLine', { b: fmtMinutes(t, status.used.browseSec), v: fmtMinutes(t, status.used.videoSec), n: r.blocked })}`;
    $('today').hidden = false;
  }
  const current = settings.ageBand || status.config.ageBand;
  $('ages').replaceChildren(...status.bands.map((b) => el('label', { class: 'kg-choice' },
    el('input', { type: 'radio', name: 'age', value: b.id, checked: b.id === current }),
    el('span', { text: t('ageYears', { range: b.id.replace('-', '–') }) }))));
  fillFor(current, settings);
  $('ages').addEventListener('change', (e) => { if (e.target.name === 'age') fillFor(e.target.value, settings.ageBand === e.target.value ? settings : null); });
  for (const k of ['browse', 'video']) $(`${k}None`).addEventListener('change', (e) => { $(`${k}Min`).disabled = e.target.checked; });
  $('bedNone').addEventListener('change', (e) => { $('bedStart').disabled = $('bedEnd').disabled = e.target.checked; });
  $('focusMode').checked = settings.focusMode !== false;
  $('adblock').checked = settings.adblock !== false;
  $('topics').checked = settings.topics !== false;
  $('allow').value = (settings.allow || []).join('\n');
  $('block').value = (settings.block || []).join('\n');
  $('form').hidden = false;
  $('form').addEventListener('submit', save);
}

async function save(e) {
  e.preventDefault();
  $('pinMsg').hidden = true; $('saveMsg').hidden = true;
  const p1 = $('pin1').value.trim(), p2 = $('pin2').value.trim();
  if (p1 || p2 || !hasPin) {
    if (!/^\d{4,8}$/.test(p1)) return fieldMsg('pinMsg', t('pinFormat'), 'pin1');
    if (p1 !== p2) return fieldMsg('pinMsg', t('pinMismatch'), 'pin2');
  }
  const ageBand = (document.querySelector('input[name="age"]:checked') || {}).value;
  const num = (id, none) => ($(none).checked ? null : $(id).value === '' ? undefined : Number($(id).value));
  const next = {
    ageBand,
    limits: { browseMin: num('browseMin', 'browseNone'), videoMin: num('videoMin', 'videoNone') },
    bedtime: $('bedNone').checked ? null : { start: $('bedStart').value, end: $('bedEnd').value },
    focusMode: $('focusMode').checked,
    adblock: $('adblock').checked,
    topics: $('topics').checked,
    allow: $('allow').value.split(/\s+/).filter(Boolean),
    block: $('block').value.split(/\s+/).filter(Boolean),
  };
  const r = await ask('settings-save', { pin, newPin: p1 || undefined, settings: next });
  const m = $('saveMsg');
  if (!r || !r.ok) { m.textContent = t(r && r.locked ? 'pinLocked' : 'saveError'); m.className = 'kg-note kg-bad'; m.hidden = false; return; }
  if (p1) { pin = p1; hasPin = true; $('pin1').value = $('pin2').value = ''; $('pinKeep').hidden = false; }
  settings = r.settings;
  $('allow').value = (settings.allow || []).join('\n');
  $('block').value = (settings.block || []).join('\n');
  m.textContent = t('saved'); m.className = 'kg-note kg-good'; m.hidden = false;
  document.title = t('optTitle'); $('title').textContent = t('optTitle');
}

function fieldMsg(id, text, focus) { $(id).textContent = text; $(id).hidden = false; $(focus).focus(); }

main();
