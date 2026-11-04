// Bedtime and day helpers. Pure functions (unit-tested in node); local machine time.

/** "20:30" -> 1230 (minutes after midnight), or null. */
export function parseHM(s) {
  const m = /^(\d{1,2}):(\d{2})$/.exec(String(s ?? '').trim());
  if (!m) return null;
  const h = Number(m[1]), mi = Number(m[2]);
  if (h > 23 || mi > 59) return null;
  return h * 60 + mi;
}

export function minuteOfDay(date) {
  return date.getHours() * 60 + date.getMinutes() + date.getSeconds() / 60;
}

/** Local calendar day "YYYY-MM-DD". */
export function localDay(date) {
  const p = (n) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}`;
}

/** Seconds since local midnight. */
export function secondsSinceMidnight(date) {
  return date.getHours() * 3600 + date.getMinutes() * 60 + date.getSeconds() + date.getMilliseconds() / 1000;
}

/** Valid bedtime object, or null when off or malformed. */
export function normBedtime(bt) {
  if (!bt || typeof bt !== 'object') return null;
  const s = parseHM(bt.start), e = parseHM(bt.end);
  if (s == null || e == null || s === e) return null;
  return { start: bt.start.trim(), end: bt.end.trim() };
}

/** True when `date` is inside bedtime. Handles a window across midnight (start > end). */
export function isBedtime(date, bt) {
  const b = normBedtime(bt);
  if (!b) return false;
  const s = parseHM(b.start), e = parseHM(b.end), m = minuteOfDay(date);
  return s < e ? (m >= s && m < e) : (m >= s || m < e);
}

/** Minutes until bedtime starts (0 while inside it), or Infinity when there is no bedtime. */
export function minutesUntilBedtime(date, bt) {
  const b = normBedtime(bt);
  if (!b) return Infinity;
  if (isBedtime(date, b)) return 0;
  const s = parseHM(b.start), m = minuteOfDay(date);
  return (s - m + 1440) % 1440;
}

// ---- clocks: local time, or a fixed UTC offset (managed mode: the agent's offset) ----
// Date.now() is UTC and a standard Windows user cannot change it; the time zone they can.
// With a fixed offset, changing the time zone neither starts a new day nor moves bedtime.

/** "2026-11-03T15:00:00+04:00" -> 240; "...Z" -> 0; null when there is no offset. */
export function parseOffsetMin(iso) {
  const m = /([+-])(\d{2}):?(\d{2})$|Z$/.exec(String(iso || '').trim());
  if (!m) return null;
  if (m[0] === 'Z') return 0;
  const v = Number(m[2]) * 60 + Number(m[3]);
  return m[1] === '-' ? -v : v;
}

/** A clock answers day(ms), minuteOfDay(ms) and secondsSinceMidnight(ms). */
export const LOCAL_CLOCK = {
  kind: 'local',
  weekday: (ms) => new Date(ms).getDay(),
  day: (ms) => localDay(new Date(ms)),
  minuteOfDay: (ms) => minuteOfDay(new Date(ms)),
  secondsSinceMidnight: (ms) => secondsSinceMidnight(new Date(ms)),
};

export function fixedClock(offsetMin) {
  const shifted = (ms) => new Date(ms + offsetMin * 60000); // read with getUTC*
  const p = (n) => String(n).padStart(2, '0');
  return {
    kind: 'fixed', offsetMin,
    weekday: (ms) => shifted(ms).getUTCDay(),
    day: (ms) => { const d = shifted(ms); return `${d.getUTCFullYear()}-${p(d.getUTCMonth() + 1)}-${p(d.getUTCDate())}`; },
    minuteOfDay: (ms) => { const d = shifted(ms); return d.getUTCHours() * 60 + d.getUTCMinutes() + d.getUTCSeconds() / 60; },
    secondsSinceMidnight: (ms) => { const d = shifted(ms); return d.getUTCHours() * 3600 + d.getUTCMinutes() * 60 + d.getUTCSeconds() + d.getUTCMilliseconds() / 1000; },
  };
}

/** Bedtime check on a clock. */
export function isBedtimeAt(ms, bt, clock = LOCAL_CLOCK) {
  const b = normBedtime(bt);
  if (!b) return false;
  const s = parseHM(b.start), e = parseHM(b.end), m = clock.minuteOfDay(ms);
  return s < e ? (m >= s && m < e) : (m >= s || m < e);
}

export function minutesUntilBedtimeAt(ms, bt, clock = LOCAL_CLOCK) {
  const b = normBedtime(bt);
  if (!b) return Infinity;
  if (isBedtimeAt(ms, b, clock)) return 0;
  return (parseHM(b.start) - clock.minuteOfDay(ms) + 1440) % 1440;
}

// ---- weekend, homework time and pause (P2-160) ----
// `sched` is the agent's /v1/state "schedule": { school: {browseMin, videoMin, bedtime}, weekend?: {...}, homework?: {start, end} }.
// The agent decides while it answers; these let the extension decide the same way when it has not answered lately.
// Weekday numbers: 0 = Sunday ... 6 = Saturday.

const DAY_MS = 86400000;
export const isWeekendDay = (wd) => wd === 0 || wd === 6;   // Saturday and Sunday minutes
export const isWeekendNight = (wd) => wd === 5 || wd === 6; // Friday and Saturday nights: a night belongs to the day it starts

const rule = (sched, weekend) => (weekend && sched && sched.weekend) || (sched && sched.school) || {};

/** Today's { browseMin, videoMin } under the schedule. */
export function limitsOn(ms, sched, clock = LOCAL_CLOCK) {
  const r = rule(sched, isWeekendDay(clock.weekday(ms)));
  return { browseMin: r.browseMin ?? null, videoMin: r.videoMin ?? null };
}

/** The bedtime of the night that starts on weekday wd. */
export function nightBedtime(wd, sched) {
  return normBedtime(rule(sched, isWeekendNight(wd)).bedtime);
}

const crossesMidnight = (b) => !!b && parseHM(b.start) > parseHM(b.end);

/** The bedtime that matters now: last night's while its morning part runs, else tonight's. */
export function bedtimeNow(ms, sched, clock = LOCAL_CLOCK) {
  const last = nightBedtime(clock.weekday(ms - DAY_MS), sched);
  if (crossesMidnight(last) && clock.minuteOfDay(ms) < parseHM(last.end)) return last;
  return nightBedtime(clock.weekday(ms), sched);
}

/** Bedtime under the schedule: last night's morning part, or tonight's window. */
export function isBedtimeSched(ms, sched, clock = LOCAL_CLOCK) {
  const m = clock.minuteOfDay(ms);
  const last = nightBedtime(clock.weekday(ms - DAY_MS), sched);
  if (crossesMidnight(last) && m < parseHM(last.end)) return true;
  const b = nightBedtime(clock.weekday(ms), sched);
  if (!b) return false;
  const s = parseHM(b.start), e = parseHM(b.end);
  return s < e ? (m >= s && m < e) : m >= s;
}

/** Homework time: inside the window on Monday to Friday. */
export function isHomeworkAt(ms, sched, clock = LOCAL_CLOCK) {
  const h = sched && sched.homework;
  if (!h) return false;
  const wd = clock.weekday(ms);
  if (wd === 0 || wd === 6) return false;
  const s = parseHM(h.start), e = parseHM(h.end), m = clock.minuteOfDay(ms);
  return s != null && e != null && s < e && m >= s && m < e;
}

/** Paused while now is before the pause's end (an RFC 3339 time from the agent). */
export function isPausedAt(ms, untilIso) {
  const end = Date.parse(String(untilIso || ''));
  return Number.isFinite(end) && ms < end;
}
