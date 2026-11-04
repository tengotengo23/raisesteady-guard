// Background entry (service worker in Chromium, event page in Firefox).
// Owns: config, time accounting, bedtime, dynamic rules, agent sync, messages kg:core:*.
// The feature modules are started from here with ctx = { getConfig, onConfig(cb), api, ready }.

import { initVideos } from './videos.js';
import { initAdblock } from './adblock.js';
import { BUILD } from './build-info.js';
import { APP_NAME } from '../ui/brand.js';
import { loadI18n, uiLang } from '../ui/i18n.js';
import { resolveConfig, publicConfig, readManaged, readLocalSettings, loadJSON, SETTINGS_KEY, bandById } from './config.js';
import { emptyUsage, observe, effectiveUsed, limitStatus, dueNotices, takeReport, settleReport, reconcileDay, addSearch } from './usage.js';
import { LOCAL_CLOCK, fixedClock, parseOffsetMin, isBedtimeAt, minutesUntilBedtimeAt, normBedtime, limitsOn, isBedtimeSched, isHomeworkAt, isPausedAt, bedtimeNow } from './schedule.js';
import { searchQuery } from './searches.js';
import { applyRules, sweepTabs, blockedUrl, contentScope } from './guard.js';
import { applyRulesets } from './safesearch.js';
import { createAgent } from './agent.js';
import { hashPin, checkPinHash, guardedCheck, PIN_RE } from './pin.js';
import { hostOf, inDomains, cleanDomain, cleanDomains, parseLearning, isLearningUrl, urlKey } from './domains.js';

const api = globalThis.browser ?? globalThis.chrome;
const EXT_ORIGIN = api.runtime.getURL('');
const RULESET_IDS = ((api.runtime.getManifest().declarative_net_request || {}).rule_resources || []).map((r) => r.id);
const TICK = 'kg-tick';
const VIDEO_FRESH_MS = 40000;
const SYNC_MS = 55000;
const AGENT_FRESH_MS = 3 * 60 * 1000; // agent answers older than this are not trusted for bedtime
const K = { usage: 'kg.usage', content: 'kg.contentDay', grants: 'kg.grants', agentState: 'kg.agentState', report: 'kg.report', pinLock: 'kg.pinLock', devManaged: 'kg.devManaged' };

// ---- state ----
let bands = [], learn = { hosts: [], paths: [] }, cfg = null, agentState = null, usage = null, t = (k) => k;
let contentDay = { day: '', domains: [], urls: [] }, grants = { day: '', extraMin: 0 }, report = { day: '', blocked: 0, byReason: {} };
let focused = true, idleState = 'active', lastSync = 0;
const videoTabs = new Map(); // 'tabId:frameId' -> { playing, at, host }
const attempted = new Map(); // tabId -> last http(s) URL the tab tried to open
const shownRecently = new Map();
const lastSearch = new Map(); // tabId -> the last search counted in that tab
const lastRequest = new Map(); // 'kind|domain' -> ms (a request at most every 10 s)
let devManaged = null; // dev builds only: a managed policy set by the e2e test hook
const dropTabVideos = (tabId) => { for (const k of [...videoTabs.keys()]) if (k.startsWith(`${tabId}:`)) videoTabs.delete(k); };
const cfgListeners = [];

// Managed mode keeps the agent's fixed UTC offset (from its last answer), so a time-zone change on
// Windows neither starts a new day nor moves bedtime. Standalone mode follows the local clock.
function clock() {
  const off = cfg && !cfg.standalone && agentState && Number.isFinite(agentState.offsetMin) ? agentState.offsetMin : null;
  return off == null ? LOCAL_CLOCK : fixedClock(off);
}
const today = () => clock().day(Date.now());
const agentFresh = () => !!(cfg && !cfg.standalone && agentState && Date.now() - (agentState.fetchedAt || 0) < AGENT_FRESH_MS);
const agent = createAgent({ api, getConfig: () => cfg });

let chain = Promise.resolve();
function serial(fn) {
  const run = chain.catch(() => {}).then(fn);
  chain = run.catch(() => {});
  return run;
}

// ---- feature hooks (registered in the first turn so their listeners can wake the worker) ----
const ctx = {
  api,
  getConfig: () => (cfg ? publicConfig(cfg) : null),
  onConfig: (cb) => { if (typeof cb === 'function') cfgListeners.push(cb); },
  ready: null,
};
for (const [name, init] of [['videos', initVideos], ['adblock', initAdblock]]) {
  try { if (typeof init === 'function') init(ctx); } catch (e) { console.warn(`[kg] ${name} init failed`, e); }
}

// ---- config ----
async function reloadConfig() {
  const managed = (BUILD.dev && devManaged) || await readManaged(api);
  const local = await readLocalSettings(api);
  const extra = grants.day === today() ? grants.extraMin : 0;
  const next = resolveConfig({ bands, managed, local, agent: agentState, uiLang: uiLang(), localExtraMin: extra });
  const pubChanged = !cfg || JSON.stringify(publicConfig(next)) !== JSON.stringify(publicConfig(cfg));
  const langChanged = !cfg || cfg.lang !== next.lang;
  cfg = next;
  if (langChanged) t = await loadI18n('core', cfg.lang);
  try { await applyRulesets(api, cfg, RULESET_IDS); } catch (e) { console.warn('[kg] rulesets', e); }
  if (pubChanged) broadcast();
}

function broadcast() {
  const msg = { type: 'kg:core:config-changed', config: publicConfig(cfg) };
  for (const cb of cfgListeners) { try { cb(msg.config); } catch { /* one listener failed: keep the others */ } }
  api.tabs.query({}).then((tabs) => {
    for (const tab of tabs) api.tabs.sendMessage(tab.id, msg).catch(() => {});
  }).catch(() => {});
  api.runtime.sendMessage(msg).catch(() => {});
}

// ---- time ----
async function currentObs() {
  let domain = null, own = false, tabUrl = null;
  if (focused) {
    try {
      const [tab] = await api.tabs.query({ active: true, lastFocusedWindow: true });
      if (tab && tab.url) { own = tab.url.startsWith(EXT_ORIGIN); domain = own ? null : hostOf(tab.url); tabUrl = own ? null : tab.url; }
    } catch { /* no window */ }
  }
  const now = Date.now();
  let video = null;
  for (const [id, v] of videoTabs) {
    if (!v.playing || now - v.at > VIDEO_FRESH_MS) { if (now - v.at > VIDEO_FRESH_MS) videoTabs.delete(id); continue; }
    const isLearn = isLearningUrl(v.url, learn);
    if (!video || (video.learn && !isLearn)) video = { host: v.host, learn: isLearn };
  }
  return {
    focused, active: idleState === 'active', domain, own, learning: !!tabUrl && isLearningUrl(tabUrl, learn),
    video: !!video, videoDomain: video ? video.host : null, videoLearning: video ? video.learn : false,
  };
}

// The agent's schedule (weekend, homework time), used while it has not answered lately (P2-160).
const agentSched = () => (cfg && !cfg.standalone && agentState && agentState.schedule && agentState.schedule.school ? agentState.schedule : null);

function guardState(nowMs = Date.now()) {
  const used = effectiveUsed(usage, agentState);
  const day = today();
  const fresh = agentFresh();
  const sched = agentSched();
  const limits = !fresh && sched ? limitsOn(nowMs, sched, clock()) : cfg.limits;
  const lim = limitStatus(used, { ...limits, extraMin: cfg.extraMin });
  const managed = !cfg.standalone && !!agentState;
  return {
    used, ...lim, limits,
    // managed and the agent answered lately: its bedtimeActive; else our clock (the agent's offset when known)
    bedtime: fresh && typeof agentState.bedtimeActive === 'boolean' ? agentState.bedtimeActive : sched ? isBedtimeSched(nowMs, sched, clock()) : isBedtimeAt(nowMs, cfg.bedtime, clock()),
    paused: managed && (fresh && typeof agentState.paused === 'boolean' ? agentState.paused : isPausedAt(nowMs, agentState.pausedUntil)),
    homework: managed && (fresh && typeof agentState.homework === 'boolean' ? agentState.homework : !!sched && isHomeworkAt(nowMs, sched, clock())),
    allow: cfg.allow, block: cfg.block, learning: learn.hosts, learningPaths: learn.paths,
    content: contentDay.day === day ? contentDay.domains : [],
    contentUrls: contentDay.day === day ? contentDay.urls || [] : [],
  };
}

async function applyGuard(s = guardState()) {
  let changed = false;
  try { changed = await applyRules(api, s); } catch (e) { console.warn('[kg] rules', e); }
  if (changed) await sweepTabs(api, s, hostOf, (tabId, url) => rememberAttempt(tabId, url));
  return s;
}

async function newDay() {
  contentDay = { day: today(), domains: [], urls: [] };
  grants = { day: today(), extraMin: 0 };
  await api.storage.local.set({ [K.content]: contentDay, [K.grants]: grants });
  await reloadConfig();
}

async function syncAgent() {
  if (cfg.standalone) return;
  lastSync = Date.now();
  const day = usage.day;
  const st = await agent.usage(takeReport(usage));
  if (!st) return;
  settleReport(usage, day);
  const before = agentState && JSON.stringify({ ...agentState, used: 0, now: 0, fetchedAt: 0, bedtimeActive: 0 });
  await setAgentState(st);
  if (before !== JSON.stringify({ ...agentState, used: 0, now: 0, fetchedAt: 0, bedtimeActive: 0 })) await reloadConfig();
}

/** Store a fresh agent answer: keep its UTC offset, and follow its day (never the local one). */
async function setAgentState(st) {
  const off = parseOffsetMin(st.now);
  agentState = { ...st, fetchedAt: Date.now(), offsetMin: off != null ? off : (agentState && agentState.offsetMin != null ? agentState.offsetMin : null) };
  usage = reconcileDay(usage, st.day);
  await api.storage.local.set({ [K.agentState]: agentState, [K.usage]: usage });
}

const NOTE_KEYS = { browse5: 'noteBrowse5', video5: 'noteVideo5', bed5: 'noteBed5', break: 'noteBreak' };
function notify(kind) {
  try {
    api.notifications.create(`kg-${kind}-${Date.now()}`, {
      type: 'basic', iconUrl: api.runtime.getURL('icons/icon-128.png'), title: APP_NAME, message: t(NOTE_KEYS[kind]),
    }).catch(() => {});
  } catch { /* notifications off by policy */ }
}

async function tick() {
  const nowMs = Date.now();
  const before = usage.day;
  usage = observe(usage, nowMs, await currentObs(), clock());
  if (!cfg.standalone && nowMs - lastSync >= SYNC_MS) await syncAgent();
  if (usage.day !== before) await newDay();
  const s = guardState(nowMs);
  const sched = agentSched();
  const bedNow = !agentFresh() && sched ? bedtimeNow(nowMs, sched, clock()) : cfg.bedtime;
  for (const kind of dueNotices(usage, s, { breakEveryMin: cfg.band.breakEveryMin, minutesToBed: minutesUntilBedtimeAt(nowMs, bedNow, clock()) })) notify(kind);
  await applyGuard(s);
  await api.storage.local.set({ [K.usage]: usage });
  return s;
}

// ---- attempts, page check, block page events ----
let attemptSave = null;
function rememberAttempt(tabId, url) {
  attempted.set(tabId, url);
  clearTimeout(attemptSave);
  attemptSave = setTimeout(() => {
    if (api.storage.session) api.storage.session.set({ 'kg.attempted': Object.fromEntries([...attempted].slice(-200)) }).catch(() => {});
  }, 500);
}

// The page check flagged this page: contentScope() (guard.js) says whether the host or only the page is remembered.
async function flagContent(sender, why) {
  const tab = sender.tab;
  const host = hostOf(tab && tab.url);
  if (!host || sender.frameId !== 0 || inDomains(host, cfg.allow)) return { ok: false };
  if (contentDay.day !== today()) contentDay = { day: today(), domains: [], urls: [] };
  contentDay.urls = contentDay.urls || [];
  if (contentScope(host, why, learn) === 'host') {
    if (!contentDay.domains.includes(host)) contentDay.domains.push(host);
    if (contentDay.domains.length > 500) contentDay.domains.shift();
  } else {
    const k = urlKey(tab.url);
    if (k && !contentDay.urls.includes(k)) contentDay.urls.push(k);
    if (contentDay.urls.length > 500) contentDay.urls.shift();
  }
  await api.storage.local.set({ [K.content]: contentDay });
  rememberAttempt(tab.id, tab.url);
  await applyGuard();
  try { await api.tabs.update(tab.id, { url: blockedUrl(api, 'content', host) }); } catch { /* closed */ }
  return { ok: true };
}

async function blockedShown(msg, sender) {
  const reason = String(msg.reason || '').slice(0, 20);
  const url = sender.tab ? attempted.get(sender.tab.id) : null;
  const host = (url && hostOf(url)) || cleanDomain(msg.site) || '';
  const key = `${sender.tab && sender.tab.id}|${reason}|${host}`;
  const now = Date.now();
  for (const [k, at] of shownRecently) if (now - at > 10000) shownRecently.delete(k);
  if (shownRecently.has(key)) return { ok: true };
  shownRecently.set(key, now);
  if (report.day !== today()) report = { day: today(), blocked: 0, byReason: {} };
  report.blocked++;
  report.byReason[reason] = (report.byReason[reason] || 0) + 1;
  await api.storage.local.set({ [K.report]: report });
  agent.event({ type: 'blocked', reason, domain: host }).catch(() => {});
  return { ok: true };
}

// ---- PIN, grants, settings ----
const lockStore = {
  get: async () => (await api.storage.local.get(K.pinLock))[K.pinLock],
  set: (v) => api.storage.local.set({ [K.pinLock]: v }),
};

async function verifyPin(pin) {
  pin = String(pin ?? '');
  if (!cfg.standalone) return agent.pin(pin);
  const local = await readLocalSettings(api);
  if (!local.pin) return { ok: false, noPin: true };
  return guardedCheck(lockStore, Date.now(), () => checkPinHash(pin, local.pin));
}

async function grant(msg) {
  const kind = msg.kind;
  const minutes = Number(msg.minutes);
  const domain = cleanDomain(msg.domain);
  if (kind === 'extra' && ![15, 30, 60].includes(minutes)) return { ok: false, error: 'minutes' };
  if (kind === 'allow' && !domain) return { ok: false, error: 'domain' };
  if (kind === 'resume' && cfg.standalone) return { ok: false, error: 'kind' };
  if (kind !== 'extra' && kind !== 'allow' && kind !== 'resume') return { ok: false, error: 'kind' };
  if (!cfg.standalone) {
    const body = kind === 'extra' ? { pin: String(msg.pin), kind, minutes } : kind === 'allow' ? { pin: String(msg.pin), kind, domain } : { pin: String(msg.pin), kind };
    const r = await agent.grant(body);
    if (r.ok && r.state) await setAgentState(r.state);
    if (r.ok) { await reloadConfig(); await serial(tick); }
    return { ok: r.ok, offline: r.offline, locked: r.locked };
  }
  const v = await verifyPin(msg.pin);
  if (!v.ok) return v;
  if (kind === 'extra') {
    const base = grants.day === today() ? grants.extraMin : 0;
    grants = { day: today(), extraMin: Math.min(base + minutes, 600) };
    await api.storage.local.set({ [K.grants]: grants });
  } else {
    const local = await readLocalSettings(api);
    local.allow = cleanDomains([...(local.allow || []), domain]);
    local.block = cleanDomains(local.block).filter((d) => d !== domain);
    await api.storage.local.set({ [SETTINGS_KEY]: local });
  }
  await reloadConfig();
  await serial(tick);
  return { ok: true };
}

// A request from the child at the block page (P2-160): sent like usage, with the account token; no PIN.
async function childRequest(msg) {
  if (cfg.standalone) return { ok: false, error: 'standalone' };
  const kind = msg.kind === 'site' ? 'site' : msg.kind === 'time' ? 'time' : null;
  const domain = kind === 'site' ? cleanDomain(msg.domain) : '';
  if (!kind || (kind === 'site' && !domain)) return { ok: false, error: 'kind' };
  const note = [...String(msg.note ?? '').replace(/[\u0000-\u001f\u007f]+/g, ' ').trim()].slice(0, 140).join('');
  const key = `${kind}|${domain}`, now = Date.now();
  if (now - (lastRequest.get(key) || 0) < 10000) return { ok: true, dup: true };
  const r = await agent.request(kind === 'site' ? { kind, domain, note } : { kind, note });
  if (r.ok) lastRequest.set(key, now);
  return r;
}

/** Count a search shown in a tab (managed mode, while the parent has searches on). */
function noteSearch(tabId, url) {
  if (!cfg || cfg.standalone || !agentState || agentState.searches !== true) return;
  const q = searchQuery(url);
  if (!q || lastSearch.get(tabId) === q) return;
  lastSearch.set(tabId, q);
  serial(async () => { usage = addSearch(usage, q); await api.storage.local.set({ [K.usage]: usage }); }).catch(() => {});
}

function settingsView(local) {
  const { pin, ...rest } = local || {};
  return { ...rest, hasPin: !!pin };
}

function validSettings(inp) {
  const out = {};
  if (!bands.some((b) => b.id === inp.ageBand)) throw new Error('ageBand');
  out.ageBand = inp.ageBand;
  const lim = (v) => (v === null ? null : Number.isFinite(Number(v)) && Number(v) >= 0 && Number(v) <= 1440 ? Math.round(Number(v)) : undefined);
  out.limits = {};
  for (const k of ['browseMin', 'videoMin']) { const v = lim(inp.limits && inp.limits[k]); if (v !== undefined) out.limits[k] = v; }
  if (inp.bedtime === null || inp.bedtime === false) out.bedtime = null;
  else if (inp.bedtime) { const b = normBedtime(inp.bedtime); if (!b) throw new Error('bedtime'); out.bedtime = b; }
  out.focusMode = inp.focusMode !== false;
  out.adblock = inp.adblock !== false;
  out.allow = cleanDomains(inp.allow).slice(0, 500);
  out.block = cleanDomains(inp.block).slice(0, 500);
  return out;
}

async function saveSettings(msg) {
  if (!cfg.standalone) return { ok: false, error: 'managed' };
  const local = await readLocalSettings(api);
  if (local.pin) {
    const v = await verifyPin(msg.pin);
    if (!v.ok) return v;
  } else if (!PIN_RE.test(String(msg.newPin ?? ''))) {
    return { ok: false, error: 'newPin' };
  }
  let next;
  try { next = validSettings(msg.settings || {}); } catch (e) { return { ok: false, error: e.message }; }
  next.pin = local.pin;
  if (msg.newPin != null && msg.newPin !== '') {
    if (!PIN_RE.test(String(msg.newPin))) return { ok: false, error: 'newPin' };
    next.pin = await hashPin(String(msg.newPin));
  }
  await api.storage.local.set({ [SETTINGS_KEY]: next });
  await reloadConfig();
  await serial(tick);
  return { ok: true, settings: settingsView(next) };
}

function finite(n) { return Number.isFinite(n) ? Math.max(0, Math.round(n)) : null; }
/** "HH:MM" of an RFC 3339 time on our clock. */
function hm(iso) {
  const ms = Date.parse(String(iso || ''));
  if (!Number.isFinite(ms)) return null;
  const m = Math.floor(clock().minuteOfDay(ms));
  return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
}

async function statusView() {
  const s = guardState();
  return {
    config: publicConfig(cfg),
    used: s.used,
    browseLeftSec: finite(s.browseLeftSec), videoLeftSec: finite(s.videoLeftSec),
    timeOver: s.timeOver, videoOver: s.videoOver, bedtimeActive: s.bedtime,
    bedtime: cfg.bedtime, limits: s.limits, extraMin: cfg.extraMin,
    paused: s.paused, pausedUntil: s.paused && agentState ? agentState.pausedUntilHM || hm(agentState.pausedUntil) : null,
    homework: s.homework, homeworkEnd: s.homework ? (agentState && (agentState.homeworkEnd || (agentState.schedule && agentState.schedule.homework && agentState.schedule.homework.end))) || null : null,
    searchesOn: !cfg.standalone && !!agentState && agentState.searches === true,
    canRequest: !cfg.standalone,
    standalone: cfg.standalone, setupDone: cfg.setupDone, hasPin: cfg.hasPin,
    parentUrl: cfg.standalone ? api.runtime.getURL('pages/options.html') : `http://127.0.0.1:${cfg.agentPort}/parent`,
    agentOnline: agent.isOnline(),
    report: report.day === today() ? report : { day: today(), blocked: 0, byReason: {} },
    bands: bands.map((b) => ({ id: b.id, browseMin: b.browseMin, videoMin: b.videoMin, bedtime: b.bedtime })),
  };
}

// ---- messages ----
const isExtPage = (sender) => sender.id === api.runtime.id && typeof sender.url === 'string' && sender.url.startsWith(EXT_ORIGIN);
const isWebFrame = (sender) => sender.id === api.runtime.id && !!sender.tab && /^https?:/.test(sender.url || '');

async function handle(msg, sender) {
  await ready;
  const type = msg.type.slice('kg:core:'.length);
  if (type === 'config') return publicConfig(cfg);
  if (isWebFrame(sender)) {
    if (type === 'video') {
      const tabId = sender.tab.id;
      const host = hostOf(sender.tab.url) || hostOf(sender.url);
      const key = `${tabId}:${sender.frameId || 0}`;
      if (msg.playing) videoTabs.set(key, { playing: true, at: Date.now(), host, url: sender.tab.url });
      else videoTabs.delete(key);
      const s = await serial(tick);
      const over = s.videoOver && !isLearningUrl(sender.tab.url, learn);
      if (over && msg.playing) {
        dropTabVideos(tabId);
        rememberAttempt(tabId, sender.tab.url);
        api.tabs.update(tabId, { url: blockedUrl(api, 'video', host) }).catch(() => {});
      }
      return { videoOver: over };
    }
    if (type === 'page-flag' && msg.reason === 'content') return serial(() => flagContent(sender, String(msg.why || 'words')));
    return { error: 'unknown' };
  }
  if (!isExtPage(sender)) return { error: 'forbidden' };
  switch (type) {
    case 'status': return statusView();
    case 'attempted': {
      const url = sender.tab ? attempted.get(sender.tab.id) || null : null;
      return { url, host: url ? hostOf(url) : null };
    }
    case 'blocked-shown': return blockedShown(msg, sender);
    case 'pin': return verifyPin(msg.pin);
    case 'grant': return grant(msg);
    case 'request': return childRequest(msg);
    case 'settings-get': {
      if (!cfg.standalone) return { ok: true, managed: true };
      const local = await readLocalSettings(api);
      if (local.pin) { const v = await verifyPin(msg.pin); if (!v.ok) return v; }
      return { ok: true, settings: settingsView(local), defaults: bandById(bands, local.ageBand || cfg.ageBand) };
    }
    case 'settings-save': return saveSettings(msg);
    case 'test': return BUILD.dev ? testHook(msg) : { error: 'forbidden' };
    default: return { error: 'unknown' };
  }
}

// Dev builds only (tools/build-ext.mjs --dev writes BUILD.dev = true): lets e2e tests set usage.
async function testHook(msg) {
  if (msg.op === 'set-usage') {
    await serial(async () => {
      usage = observe(usage, Date.now(), undefined, clock());
      for (const k of ['browseSec', 'videoSec', 'learningSec']) if (Number.isFinite(msg[k])) usage[k] = msg[k];
      await api.storage.local.set({ [K.usage]: usage });
    });
  }
  if (msg.op === 'set-settings') { await api.storage.local.set({ [SETTINGS_KEY]: msg.settings || {} }); await reloadConfig(); }
  if (msg.op === 'set-managed') {
    devManaged = msg.managed || null;
    await api.storage.local.set({ [K.devManaged]: devManaged });
    if (!devManaged) { agentState = null; await api.storage.local.remove(K.agentState); }
    await reloadConfig();
  }
  if (msg.op === 'sync') { lastSync = 0; }
  const s = await serial(tick);
  return { ok: true, usage, timeOver: s.timeOver, videoOver: s.videoOver, bedtime: s.bedtime, paused: s.paused, homework: s.homework, agentOnline: agent.isOnline(), cfg: publicConfig(cfg) };
}

api.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || typeof msg.type !== 'string' || !msg.type.startsWith('kg:core:') || msg.type === 'kg:core:config-changed') return false;
  handle(msg, sender).then(sendResponse, (e) => sendResponse({ error: String((e && e.message) || e) }));
  return true;
});

// ---- events ----
const kick = () => { ready.then(() => serial(tick)).catch(() => {}); };
api.alarms.onAlarm.addListener((a) => { if (a.name === TICK) kick(); });
api.windows.onFocusChanged.addListener((id) => { focused = id !== api.windows.WINDOW_ID_NONE; kick(); });
api.idle.onStateChanged.addListener((st) => { idleState = st; kick(); });
api.tabs.onActivated.addListener(kick);
api.tabs.onUpdated.addListener((id, info) => {
  if (info.url) ready.then(() => noteSearch(id, info.url)).catch(() => {});
  if (info.url || info.status === 'complete') kick();
});
api.tabs.onRemoved.addListener((id) => { dropTabVideos(id); attempted.delete(id); lastSearch.delete(id); kick(); });
api.webNavigation.onBeforeNavigate.addListener((d) => {
  if (d.frameId === 0 && /^https?:/.test(d.url)) rememberAttempt(d.tabId, d.url);
});
api.storage.onChanged.addListener((changes, area) => {
  if (area === 'managed' || (area === 'local' && changes[SETTINGS_KEY])) ready.then(() => serial(async () => { await reloadConfig(); await tick(); })).catch(() => {});
});
api.runtime.onInstalled.addListener((d) => {
  ready.then(() => { if (d.reason === 'install' && cfg.standalone && !BUILD.dev) api.runtime.openOptionsPage().catch(() => {}); }).catch(() => {});
});

// ---- start ----
const ready = (async () => {
  bands = ((await loadJSON(api, 'data/age-bands.json', { bands: [] })) || {}).bands || [];
  learn = parseLearning(((await loadJSON(api, 'data/learning-domains.json', { domains: [] })) || {}).domains);
  const st = await api.storage.local.get(Object.values(K));
  usage = st[K.usage] && st[K.usage].pending ? st[K.usage] : emptyUsage(today());
  contentDay = st[K.content] || contentDay;
  grants = st[K.grants] || grants;
  report = st[K.report] || report;
  agentState = st[K.agentState] || null;
  if (BUILD.dev) devManaged = st[K.devManaged] || null;
  if (api.storage.session) {
    try { const s = (await api.storage.session.get('kg.attempted'))['kg.attempted'] || {}; for (const [k, v] of Object.entries(s)) attempted.set(Number(k), v); } catch { /* none */ }
  }
  await reloadConfig();
  try { api.idle.setDetectionInterval(60); idleState = await api.idle.queryState(60); } catch { /* keep active */ }
  try { const w = await api.windows.getLastFocused(); focused = !!(w && w.focused); } catch { /* keep */ }
  if (!(await api.alarms.get(TICK))) api.alarms.create(TICK, { periodInMinutes: 0.5 });
  await serial(tick);
})();
ctx.ready = ready;
ready.catch((e) => console.warn('[kg] start failed', e));
