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
  day: (ms) => localDay(new Date(ms)),
  minuteOfDay: (ms) => minuteOfDay(new Date(ms)),
  secondsSinceMidnight: (ms) => secondsSinceMidnight(new Date(ms)),
};

export function fixedClock(offsetMin) {
  const shifted = (ms) => new Date(ms + offsetMin * 60000); // read with getUTC*
  const p = (n) => String(n).padStart(2, '0');
  return {
    kind: 'fixed', offsetMin,
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
