// Time accounting (pure; unit-tested). The background feeds observations; this module adds the
// seconds since the previous observation to the right counters.
//
// Counting rules (brief): browsing counts while a browser window has focus and the user is active
// (idle after 60 s), or while a video plays. Video time counts while a video plays (audibly or
// visibly) in any tab. Time on learning domains counts as learning, not browsing or video.
// The extension's own pages (new tab, block page) do not count. A new local day starts at midnight.

import { LOCAL_CLOCK } from './schedule.js';

export const MAX_GAP_SEC = 120;        // a longer gap means sleep or a stopped browser: count nothing
export const BREAK_RESET_SEC = 5 * 60; // 5 minutes away from the screen resets the break timer
export const WARN_SEC = 5 * 60;        // warn 5 minutes before a limit
const MAX_DOMAINS = 400;
export const MAX_PENDING_SEARCHES = 50;

const zero = () => ({ browseSec: 0, videoSec: 0, learningSec: 0, domains: {} });

export function emptyUsage(day) {
  return { day, ...zero(), pending: zero(), lastMs: null, obs: null, contSec: 0, lastUseMs: null, warned: {} };
}

/** Observation shape: { focused, active, domain, learning, own, video, videoDomain, videoLearning }. */
export function emptyObs() {
  return { focused: false, active: false, domain: null, learning: false, own: false, video: false, videoDomain: null, videoLearning: false };
}

function add(u, field, dt, domain) {
  u[field] += dt;
  u.pending[field] += dt;
  if (domain && field === 'browseSec') {
    for (const bag of [u.domains, u.pending.domains]) {
      if (bag[domain] != null || Object.keys(bag).length < MAX_DOMAINS) bag[domain] = (bag[domain] || 0) + dt;
    }
  }
}

function credit(u, o, dt, nowMs) {
  if (!o || dt <= 0) return;
  const videoCounts = !!(o.video && !o.videoLearning);
  const foreground = !!(o.focused && o.active && o.domain && !o.own);
  if (videoCounts) {
    add(u, 'browseSec', dt, o.videoDomain || o.domain);
    add(u, 'videoSec', dt);
  } else if (foreground && !o.learning) {
    add(u, 'browseSec', dt, o.domain);
  } else if ((foreground && o.learning) || (o.video && o.videoLearning)) {
    add(u, 'learningSec', dt);
  }
  const using = videoCounts || foreground || !!o.video;
  if (using) { u.contSec += dt; u.lastUseMs = nowMs; }
}

/**
 * Add the time since the last call under the previous observation, then store the new one.
 * Returns the (possibly new-day) usage object. `obs` may be omitted to just accrue.
 * `clock` decides the day (LOCAL_CLOCK, or fixedClock(agent offset) in managed mode). A day only
 * moves forward: if the clock says an earlier day (time zone moved back), counters are kept.
 */
export function observe(u, nowMs, obs, clock = LOCAL_CLOCK) {
  const today = clock.day(nowMs);
  let dt = u.lastMs == null ? 0 : (nowMs - u.lastMs) / 1000;
  if (!(dt > 0) || dt > MAX_GAP_SEC) dt = 0;
  if (today > u.day) {
    // New day: the part of the gap after midnight goes to the new day; the rest is dropped.
    const fresh = emptyUsage(today);
    fresh.obs = u.obs;
    dt = Math.min(dt, clock.secondsSinceMidnight(nowMs));
    u = fresh;
  }
  credit(u, u.obs, dt, nowMs);
  if (u.lastUseMs != null && nowMs - u.lastUseMs > BREAK_RESET_SEC * 1000) u.contSec = 0;
  u.lastMs = nowMs;
  if (obs) u.obs = { ...emptyObs(), ...obs };
  return u;
}

/**
 * Managed mode: the agent's day is the day. Agent ahead: start that day. Agent behind (our clock ran
 * ahead, for example after a time-zone change): go back to the agent's day and keep our counters, so
 * effectiveUsed() takes the larger of ours and the agent's. Never a reset because of the local day.
 */
export function reconcileDay(u, agentDay) {
  if (!agentDay || !/^\d{4}-\d{2}-\d{2}$/.test(agentDay) || agentDay === u.day) return u;
  if (agentDay > u.day) {
    const fresh = emptyUsage(agentDay);
    fresh.obs = u.obs; fresh.lastMs = u.lastMs; fresh.lastUseMs = u.lastUseMs; fresh.contSec = u.contSec;
    return fresh;
  }
  u.day = agentDay;
  return u;
}

/** Today's totals across all browsers: the agent's count plus what we have not sent yet, never below our own count. */
export function effectiveUsed(u, agentState) {
  const own = { browseSec: u.browseSec, videoSec: u.videoSec, learningSec: u.learningSec };
  const a = agentState && agentState.day === u.day && agentState.used ? agentState.used : null;
  if (!a) return own;
  return {
    browseSec: Math.max(own.browseSec, (a.browseSec || 0) + u.pending.browseSec),
    videoSec: Math.max(own.videoSec, (a.videoSec || 0) + u.pending.videoSec),
    learningSec: Math.max(own.learningSec, (a.learningSec || 0) + u.pending.learningSec),
  };
}

const limitSec = (min, extraMin) => (min == null ? Infinity : Math.max(0, (Number(min) + Number(extraMin || 0)) * 60));

/** Limits state. browseMin / videoMin null = no limit. extraMin (granted today) applies to both. */
export function limitStatus(used, { browseMin, videoMin, extraMin = 0 }) {
  const bl = limitSec(browseMin, extraMin), vl = limitSec(videoMin, extraMin);
  const browseLeftSec = bl - used.browseSec, videoLeftSec = vl - used.videoSec;
  return {
    browseLeftSec, videoLeftSec,
    timeOver: browseLeftSec <= 0,
    videoOver: videoLeftSec <= 0,
  };
}

/**
 * Which reminders to show now; marks them in u.warned so each shows once a day
 * (the break reminder repeats after every `breakEveryMin` of continuous use).
 */
export function dueNotices(u, st, { breakEveryMin, minutesToBed = Infinity }) {
  const out = [];
  const once = (key) => { if (!u.warned[key]) { u.warned[key] = true; out.push(key); } };
  if (st.browseLeftSec > 0 && st.browseLeftSec <= WARN_SEC) once('browse5');
  if (st.videoLeftSec > 0 && st.videoLeftSec <= WARN_SEC && st.browseLeftSec > WARN_SEC) once('video5');
  if (minutesToBed > 0 && minutesToBed <= 5) once('bed5');
  if (breakEveryMin > 0 && u.contSec >= breakEveryMin * 60) { u.contSec = 0; out.push('break'); }
  return out;
}

/** Count one search for the next report (P2-160). The agent keeps them only while the parent has them on. */
export function addSearch(u, q) {
  if (!q) return u;
  const bag = u.pending.searches || (u.pending.searches = {});
  if (bag[q] != null || Object.keys(bag).length < MAX_PENDING_SEARCHES) bag[q] = (bag[q] || 0) + 1;
  return u;
}

/** Take up to `max` seconds per field from pending for one agent report; the rest is dropped (the agent caps it anyway). */
export function takeReport(u, max = 120) {
  const p = u.pending;
  const body = {
    browseSec: Math.min(max, Math.round(p.browseSec)),
    videoSec: Math.min(max, Math.round(p.videoSec)),
    learningSec: Math.min(max, Math.round(p.learningSec)),
    domains: {},
  };
  for (const [d, s] of Object.entries(p.domains)) { const r = Math.round(s); if (r > 0) body.domains[d] = Math.min(max, r); }
  const searches = Object.entries(p.searches || {}).slice(0, 20);
  if (searches.length) body.searches = Object.fromEntries(searches);
  return body;
}

/** After the agent accepted a report: drop what was sent (and anything above the per-call cap). */
export function settleReport(u, sentDay) {
  if (u.day === sentDay) u.pending = zero();
  return u;
}
