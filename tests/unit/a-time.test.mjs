// usage accounting and bedtime (local time).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { emptyUsage, observe, effectiveUsed, limitStatus, dueNotices, takeReport, settleReport, MAX_GAP_SEC } from '../../ext/background/usage.js';
import { isBedtime, minutesUntilBedtime, parseHM, localDay, normBedtime } from '../../ext/background/schedule.js';

const at = (y, mo, d, h, mi, s = 0) => new Date(y, mo - 1, d, h, mi, s).getTime();
const T0 = at(2026, 11, 3, 15, 0);
const fg = (domain, extra = {}) => ({ focused: true, active: true, domain, learning: false, own: false, video: false, ...extra });

function run(steps) {
  let u = emptyUsage(localDay(new Date(steps[0][0])));
  for (const [ms, obs] of steps) u = observe(u, ms, obs);
  return u;
}

test('foreground browsing counts per second and per domain', () => {
  const u = run([[T0, fg('example.org')], [T0 + 30e3, fg('example.org')], [T0 + 60e3, fg('news.example')], [T0 + 90e3]]);
  assert.equal(u.browseSec, 90);
  assert.equal(u.videoSec, 0);
  assert.deepEqual(u.domains, { 'example.org': 60, 'news.example': 30 });
  assert.equal(u.pending.browseSec, 90);
});

test('idle, unfocused and own pages do not count', () => {
  const u = run([
    [T0, fg('a.example', { active: false })],
    [T0 + 30e3, { ...fg('a.example'), focused: false }],
    [T0 + 60e3, fg(null, { own: true })],
    [T0 + 90e3],
  ]);
  assert.equal(u.browseSec, 0);
});

test('a playing video counts as browsing and video, even without focus', () => {
  const u = run([[T0, { focused: false, active: false, video: true, videoDomain: 'youtube.com' }], [T0 + 45e3]]);
  assert.equal(u.browseSec, 45);
  assert.equal(u.videoSec, 45);
  assert.equal(u.domains['youtube.com'], 45);
});

test('video and foreground at once count once', () => {
  const u = run([[T0, fg('example.org', { video: true, videoDomain: 'youtube.com' })], [T0 + 30e3]]);
  assert.equal(u.browseSec, 30);
  assert.equal(u.videoSec, 30);
});

test('learning domains count as learning only (also their videos)', () => {
  const u = run([
    [T0, fg('khanacademy.org', { learning: true })],
    [T0 + 60e3, { focused: false, active: true, video: true, videoDomain: 'khanacademy.org', videoLearning: true }],
    [T0 + 90e3],
  ]);
  assert.equal(u.browseSec, 0);
  assert.equal(u.videoSec, 0);
  assert.equal(u.learningSec, 90);
});

test('a gap longer than MAX_GAP (sleep) is not counted', () => {
  const u = run([[T0, fg('a.example')], [T0 + (MAX_GAP_SEC + 1) * 1000, fg('a.example')], [T0 + (MAX_GAP_SEC + 31) * 1000]]);
  assert.equal(u.browseSec, 30);
});

test('day rollover at local midnight: only the part after midnight counts for the new day', () => {
  const before = at(2026, 11, 3, 23, 59, 30);
  const after = at(2026, 11, 4, 0, 0, 20);
  let u = emptyUsage('2026-11-03');
  u = observe(u, before - 60e3, fg('a.example'));
  u = observe(u, before);
  assert.equal(u.browseSec, 60);
  u = observe(u, after);
  assert.equal(u.day, '2026-11-04');
  assert.equal(u.browseSec, 20);
  assert.equal(u.pending.browseSec, 20);
  assert.deepEqual(u.warned, {});
});

test('effective used: agent total plus unsent, never below our own count', () => {
  const u = emptyUsage('2026-11-03');
  u.browseSec = 600; u.pending.browseSec = 60;
  assert.equal(effectiveUsed(u, { day: '2026-11-03', used: { browseSec: 3000, videoSec: 0, learningSec: 0 } }).browseSec, 3060);
  assert.equal(effectiveUsed(u, { day: '2026-11-03', used: { browseSec: 100, videoSec: 0, learningSec: 0 } }).browseSec, 600);
  assert.equal(effectiveUsed(u, { day: '2026-11-02', used: { browseSec: 9999 } }).browseSec, 600, 'stale agent day ignored');
  assert.equal(effectiveUsed(u, null).browseSec, 600);
});

test('limits: over at the limit, extra minutes extend both, null = no limit', () => {
  assert.equal(limitStatus({ browseSec: 5400, videoSec: 0 }, { browseMin: 90, videoMin: 45 }).timeOver, true);
  assert.equal(limitStatus({ browseSec: 5399, videoSec: 0 }, { browseMin: 90, videoMin: 45 }).timeOver, false);
  const st = limitStatus({ browseSec: 5400, videoSec: 2700 }, { browseMin: 90, videoMin: 45, extraMin: 15 });
  assert.equal(st.timeOver, false); assert.equal(st.videoOver, false); assert.equal(st.browseLeftSec, 900);
  const none = limitStatus({ browseSec: 99999, videoSec: 99999 }, { browseMin: null, videoMin: null });
  assert.equal(none.timeOver, false); assert.equal(none.browseLeftSec, Infinity);
  assert.equal(limitStatus({ browseSec: 0, videoSec: 0 }, { browseMin: 0, videoMin: 0 }).timeOver, true, '0 minutes blocks');
});

test('reminders: 5-minute warning once, break after breakEveryMin, bedtime soon', () => {
  const u = emptyUsage('2026-11-03');
  const st = limitStatus({ browseSec: 90 * 60 - 200, videoSec: 0 }, { browseMin: 90, videoMin: 45 });
  assert.deepEqual(dueNotices(u, st, { breakEveryMin: 30 }), ['browse5']);
  assert.deepEqual(dueNotices(u, st, { breakEveryMin: 30 }), [], 'only once');
  u.contSec = 30 * 60;
  assert.deepEqual(dueNotices(u, st, { breakEveryMin: 30, minutesToBed: 4 }), ['bed5', 'break']);
  assert.equal(u.contSec, 0);
});

test('break timer resets after 5 minutes away', () => {
  let u = emptyUsage(localDay(new Date(T0)));
  u = observe(u, T0, fg('a.example'));
  u = observe(u, T0 + 60e3, fg(null, { focused: false }));
  assert.equal(u.contSec, 60);
  for (let i = 1; i <= 11; i++) u = observe(u, T0 + 60e3 + i * 30e3, fg(null, { focused: false }));
  assert.equal(u.contSec, 0);
});

test('agent report caps each field at 120 s and settles pending', () => {
  const u = emptyUsage('2026-11-03');
  u.pending = { browseSec: 300, videoSec: 20.4, learningSec: 0, domains: { 'a.example': 300 } };
  assert.deepEqual(takeReport(u), { browseSec: 120, videoSec: 20, learningSec: 0, domains: { 'a.example': 120 } });
  settleReport(u, '2026-11-03');
  assert.equal(u.pending.browseSec, 0);
});

test('bedtime across midnight and inside one day', () => {
  const bt = { start: '20:00', end: '07:00' };
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 19, 59)), bt), false);
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 20, 0)), bt), true);
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 23, 59)), bt), true);
  assert.equal(isBedtime(new Date(at(2026, 11, 4, 0, 0)), bt), true);
  assert.equal(isBedtime(new Date(at(2026, 11, 4, 6, 59)), bt), true);
  assert.equal(isBedtime(new Date(at(2026, 11, 4, 7, 0)), bt), false);
  const nap = { start: '13:00', end: '14:30' };
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 13, 30)), nap), true);
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 14, 30)), nap), false);
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 2, 0)), nap), false);
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 21, 0)), null), false);
  assert.equal(isBedtime(new Date(at(2026, 11, 3, 21, 0)), { start: '20:00', end: '20:00' }), false, 'empty window is off');
});

test('minutes until bedtime, also across midnight', () => {
  assert.equal(Math.round(minutesUntilBedtime(new Date(at(2026, 11, 3, 19, 55)), { start: '20:00', end: '07:00' })), 5);
  assert.equal(minutesUntilBedtime(new Date(at(2026, 11, 3, 23, 0)), { start: '20:00', end: '07:00' }), 0);
  assert.equal(Math.round(minutesUntilBedtime(new Date(at(2026, 11, 3, 23, 50)), { start: '00:10', end: '06:00' })), 20);
  assert.equal(minutesUntilBedtime(new Date(), null), Infinity);
});

test('time parsing', () => {
  assert.equal(parseHM('07:00'), 420);
  assert.equal(parseHM('7:05'), 425);
  assert.equal(parseHM('24:00'), null);
  assert.equal(parseHM('ab'), null);
  assert.equal(normBedtime({ start: '25:00', end: '07:00' }), null);
});
