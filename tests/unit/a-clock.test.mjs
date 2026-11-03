// the time-zone trick. Managed mode keeps the agent's day and UTC offset; a time-zone change
// on Windows (allowed for standard users) must not start a new day, reset counters or move bedtime.
import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { emptyUsage, observe, reconcileDay, effectiveUsed, limitStatus } from '../../ext/background/usage.js';
import { LOCAL_CLOCK, fixedClock, parseOffsetMin, isBedtimeAt, minutesUntilBedtimeAt } from '../../ext/background/schedule.js';

const TZ0 = process.env.TZ;
after(() => { if (TZ0 === undefined) delete process.env.TZ; else process.env.TZ = TZ0; });
const fg = { focused: true, active: true, domain: 'a.example', learning: false, own: false, video: false };
// 2026-11-03 21:30 in Tbilisi (UTC+4) = 17:30 UTC
const T = Date.UTC(2026, 10, 3, 17, 30);
const TBILISI = fixedClock(240);

test('agent offset parsing', () => {
  assert.equal(parseOffsetMin('2026-11-03T21:30:00+04:00'), 240);
  assert.equal(parseOffsetMin('2026-11-03T21:30:00-0530'), -330);
  assert.equal(parseOffsetMin('2026-11-03T17:30:00Z'), 0);
  assert.equal(parseOffsetMin('2026-11-03T17:30:00'), null);
  assert.equal(TBILISI.day(T), '2026-11-03');
  assert.equal(TBILISI.minuteOfDay(T), 21 * 60 + 30);
});

test('zone moved forward past midnight: the fixed clock keeps the day and the counters', () => {
  process.env.TZ = 'Asia/Tbilisi';
  let u = emptyUsage(TBILISI.day(T));
  u = observe(u, T, fg, TBILISI);
  u = observe(u, T + 30e3, fg, TBILISI);
  assert.equal(u.browseSec, 30);
  process.env.TZ = 'Pacific/Kiritimati'; // UTC+14: local clock now says 4 Nov, 07:30
  assert.equal(LOCAL_CLOCK.day(T + 60e3), '2026-11-04', 'the local clock was fooled');
  u = observe(u, T + 60e3, fg, TBILISI);
  assert.equal(u.day, '2026-11-03');
  assert.equal(u.browseSec, 60, 'no fresh day');
  assert.equal(isBedtimeAt(T + 60e3, { start: '21:00', end: '07:00' }, TBILISI), true, 'bedtime does not move');
  assert.equal(isBedtimeAt(T + 60e3, { start: '21:00', end: '07:00' }, LOCAL_CLOCK), false, '(the local clock would have ended it)');
});

test('zone moved back: no new day and bedtime stays', () => {
  process.env.TZ = 'Pacific/Honolulu'; // UTC-10: local 07:30 on 3 Nov
  let u = emptyUsage('2026-11-03');
  u = observe(u, T, fg, TBILISI);
  u = observe(u, T + 45e3, fg, TBILISI);
  assert.equal(u.day, '2026-11-03');
  assert.equal(u.browseSec, 45);
  assert.equal(isBedtimeAt(T, { start: '21:00', end: '07:00' }, TBILISI), true);
  assert.equal(minutesUntilBedtimeAt(T - 3600e3, { start: '21:00', end: '07:00' }, TBILISI), 30);
});

test('standalone (local clock): a day never goes backwards, counters are kept', () => {
  process.env.TZ = 'Asia/Tbilisi';
  let u = emptyUsage(LOCAL_CLOCK.day(T));
  u = observe(u, T, fg);
  u = observe(u, T + 30e3, fg);
  process.env.TZ = 'Pacific/Pago_Pago'; // UTC-11: local clock now says 3 Nov 06:30, before bedtime
  u = observe(u, T + 60e3, fg);
  assert.equal(u.browseSec, 60);
  const back = emptyUsage('2026-11-04');
  back.browseSec = 500;
  const r = observe(back, T, fg); // local day 2026-11-03 < 2026-11-04
  assert.equal(r.day, '2026-11-04');
  assert.equal(r.browseSec, 500);
});

test('agent day ahead: start that day; agent day behind: take it, keep counters', () => {
  const u = emptyUsage('2026-11-03');
  u.browseSec = 4000; u.lastMs = T;
  const ahead = reconcileDay(u, '2026-11-04');
  assert.equal(ahead.day, '2026-11-04');
  assert.equal(ahead.browseSec, 0);
  const v = emptyUsage('2026-11-04');
  v.browseSec = 300;
  const behind = reconcileDay(v, '2026-11-03');
  assert.equal(behind.day, '2026-11-03');
  assert.equal(behind.browseSec, 300);
  const agent = { day: '2026-11-03', used: { browseSec: 6900, videoSec: 0, learningSec: 0 } };
  assert.equal(effectiveUsed(behind, agent).browseSec, 6900, 'agent total counts again');
  assert.equal(reconcileDay(v, 'garbage'), v);
  assert.equal(reconcileDay(v, undefined), v);
});

test('agent down then up: local counting goes on, then the larger total wins', () => {
  let u = emptyUsage('2026-11-03');
  u = observe(u, T, fg, TBILISI);
  const agentBefore = { day: '2026-11-03', used: { browseSec: 7000, videoSec: 0, learningSec: 0 } };
  // agent unreachable for 10 minutes: we keep counting on our own
  for (let i = 1; i <= 20; i++) u = observe(u, T + i * 30e3, fg, TBILISI);
  assert.equal(u.browseSec, 600);
  assert.equal(effectiveUsed(u, agentBefore).browseSec, 7600, 'old agent total plus what it has not seen');
  assert.equal(limitStatus(effectiveUsed(u, agentBefore), { browseMin: 120, videoMin: 60 }).timeOver, true);
  // agent back, but it lost time (capped report): ours is larger and stays
  const agentAfter = { day: '2026-11-03', used: { browseSec: 120, videoSec: 0, learningSec: 0 } };
  u.pending.browseSec = 0;
  assert.equal(effectiveUsed(u, agentAfter).browseSec, 600);
});
