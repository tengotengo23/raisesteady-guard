// P2-160: weekend times, homework time, pause, search words, and their block rules.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fixedClock, LOCAL_CLOCK, limitsOn, nightBedtime, bedtimeNow, isBedtimeSched, isHomeworkAt, isPausedAt, isWeekendDay, isWeekendNight } from '../../ext/background/schedule.js';
import { searchQuery, cleanSearch, MAX_SEARCH_LEN } from '../../ext/background/searches.js';
import { emptyUsage, addSearch, takeReport, settleReport, MAX_PENDING_SEARCHES } from '../../ext/background/usage.js';
import { buildRules, reasonFor, RULE_ID } from '../../ext/background/guard.js';

// The agent's clock: UTC+04:00. 2026-11-02 is a Monday.
const clock = fixedClock(240);
const at = (day, h, m = 0) => Date.UTC(2026, 10, day, h - 4, m);
const sched = {
  school: { browseMin: 120, videoMin: 60, bedtime: { start: '21:00', end: '07:00' } },
  weekend: { browseMin: 180, videoMin: 90, bedtime: { start: '22:30', end: '09:00' } },
  homework: { start: '16:00', end: '18:00' },
};
const noWeekend = { school: sched.school };

test('weekday on the agent clock, not the computer clock', () => {
  assert.equal(clock.weekday(at(7, 1, 0)), 6, 'Saturday 01:00 at UTC+4 is still Friday 21:00 UTC');
  assert.equal(fixedClock(0).weekday(at(7, 1, 0)), 5);
  assert.equal(typeof LOCAL_CLOCK.weekday(Date.now()), 'number');
  assert.deepEqual([0, 1, 5, 6].map(isWeekendDay), [true, false, false, true]);
  assert.deepEqual([0, 4, 5, 6].map(isWeekendNight), [false, false, true, true]);
});

test('weekend minutes on Saturday and Sunday only', () => {
  assert.deepEqual(limitsOn(at(6, 12), sched, clock), { browseMin: 120, videoMin: 60 }, 'Friday');
  assert.deepEqual(limitsOn(at(7, 12), sched, clock), { browseMin: 180, videoMin: 90 }, 'Saturday');
  assert.deepEqual(limitsOn(at(8, 23), sched, clock), { browseMin: 180, videoMin: 90 }, 'Sunday night');
  assert.deepEqual(limitsOn(at(9, 0, 30), sched, clock), { browseMin: 120, videoMin: 60 }, 'Monday just after midnight');
  assert.deepEqual(limitsOn(at(7, 12), noWeekend, clock), { browseMin: 120, videoMin: 60 }, 'no weekend rule');
});

test('Friday night uses the weekend bedtime, Sunday night the school one', () => {
  const cases = [
    ['Thursday 21:30', at(5, 21, 30), true],
    ['Friday 21:30 (weekend night starts 22:30)', at(6, 21, 30), false],
    ['Friday 22:45', at(6, 22, 45), true],
    ['Saturday 08:00: end of Friday night', at(7, 8), true],
    ['Saturday 09:30', at(7, 9, 30), false],
    ['Saturday 22:00', at(7, 22), false],
    ['Sunday 08:30: end of Saturday night', at(8, 8, 30), true],
    ['Sunday 21:15: Sunday night is a school night', at(8, 21, 15), true],
    ['Monday 06:59', at(9, 6, 59), true],
    ['Monday 07:30', at(9, 7, 30), false],
  ];
  for (const [name, ms, want] of cases) assert.equal(isBedtimeSched(ms, sched, clock), want, name);
  assert.deepEqual(nightBedtime(5, sched), sched.weekend.bedtime);
  assert.deepEqual(nightBedtime(0, sched), sched.school.bedtime);
  assert.equal(bedtimeNow(at(8, 8), sched, clock).end, '09:00', 'Sunday morning shows Saturday night\'s end');
  assert.equal(bedtimeNow(at(8, 12), sched, clock).start, '21:00', 'Sunday afternoon shows tonight\'s school bedtime');
  assert.equal(isBedtimeSched(at(7, 22), noWeekend, clock), true, 'without a weekend rule Saturday uses the school bedtime');
});

test('homework time: inside the window on school days only', () => {
  assert.equal(isHomeworkAt(at(4, 16, 30), sched, clock), true, 'Wednesday');
  assert.equal(isHomeworkAt(at(4, 18, 0), sched, clock), false, 'the end is outside');
  assert.equal(isHomeworkAt(at(4, 15, 59), sched, clock), false);
  assert.equal(isHomeworkAt(at(6, 17), sched, clock), true, 'Friday');
  assert.equal(isHomeworkAt(at(7, 17), sched, clock), false, 'Saturday');
  assert.equal(isHomeworkAt(at(8, 17), sched, clock), false, 'Sunday');
  assert.equal(isHomeworkAt(at(4, 17), noWeekend, clock), false, 'no window');
  assert.equal(isHomeworkAt(at(4, 17), { ...sched, homework: { start: '18:00', end: '16:00' } }, clock), false, 'a window backwards never applies');
});

test('pause: until its end time, whatever the clock zone', () => {
  const until = '2026-11-04T17:30:00+04:00';
  assert.equal(isPausedAt(at(4, 17, 29), until), true);
  assert.equal(isPausedAt(at(4, 17, 30), until), false);
  assert.equal(isPausedAt(at(4, 12), ''), false);
  assert.equal(isPausedAt(at(4, 12), 'soon'), false);
});

test('search words from each engine (address only)', () => {
  const cases = [
    ['https://www.google.com/search?q=How+do+Volcanoes+work&hl=en', 'how do volcanoes work'],
    ['https://www.google.co.uk/search?q=fractions', 'fractions'],
    ['https://www.bing.com/search?q=Solar%20System&form=QBLH', 'solar system'],
    ['https://www.bing.com/videos/search?q=cats', 'cats'],
    ['https://duckduckgo.com/?q=Photosynthesis&ia=web', 'photosynthesis'],
    ['https://html.duckduckgo.com/html/?q=maths', 'maths'],
    ['https://search.yahoo.com/search?p=Dinosaurs', 'dinosaurs'],
    ['https://uk.search.yahoo.com/search?p=rain', 'rain'],
    ['https://www.ecosia.org/search?method=index&q=Trees', 'trees'],
    ['https://yandex.ru/search/?text=%D0%BA%D0%BE%D1%88%D0%BA%D0%B8', 'кошки'],
    ['https://ya.ru/search/?text=Weather', 'weather'],
    ['https://www.youtube.com/results?search_query=Minecraft+Build', 'minecraft build'],
    ['https://m.youtube.com/results?search_query=lego', 'lego'],
  ];
  for (const [url, want] of cases) assert.equal(searchQuery(url), want, url);
  for (const url of [
    'https://www.google.com/maps?q=tbilisi', 'https://mail.google.com/search?q=x', 'https://www.youtube.com/watch?v=abc',
    'https://example.org/search?q=hello', 'https://www.google.com/search?q=+++', 'chrome://settings/?q=x', 'not a url',
    'https://www.bing.com/?q=home',
  ]) assert.equal(searchQuery(url), null, url);
  assert.equal([...cleanSearch('ა'.repeat(150))].length, MAX_SEARCH_LEN, 'at most 100 characters');
  assert.equal(cleanSearch('  A\tb\nC  '), 'a b c');
});

test('searches go with the next usage report and leave after it', () => {
  let u = emptyUsage('2026-11-04');
  delete u.pending.searches; // a 0.2.2 usage object from storage
  u = addSearch(u, 'volcano');
  u = addSearch(u, 'volcano');
  u = addSearch(u, null);
  assert.deepEqual(takeReport(u).searches, { volcano: 2 });
  for (let i = 0; i < 80; i++) addSearch(u, `q${i}`);
  assert.equal(Object.keys(u.pending.searches).length, MAX_PENDING_SEARCHES);
  assert.equal(Object.keys(takeReport(u).searches).length, 20, 'at most 20 a report (the agent takes 20)');
  settleReport(u, '2026-11-04');
  assert.equal(takeReport(u).searches, undefined, 'none left after the report');
});

test('rules: pause blocks everything, homework only non-learning sites', () => {
  const learning = ['khanacademy.org'];
  const r = buildRules({ paused: true, homework: true, allow: ['school.example', 'bad.example'], block: ['bad.example'], learning, learningPaths: [{ host: 'bbc.co.uk', path: '/bitesize' }] });
  const by = Object.fromEntries(r.map((x) => [x.id, x]));
  assert.equal(by[RULE_ID.paused].priority, 400);
  assert.equal(by[RULE_ID.paused].action.redirect.extensionPath, '/pages/blocked.html?reason=paused');
  assert.deepEqual(by[RULE_ID.paused].condition.excludedRequestDomains, ['127.0.0.1', 'localhost'], 'only the parent page stays open while paused');
  const hw = by[RULE_ID.homework];
  assert.equal(hw.action.redirect.extensionPath, '/pages/blocked.html?reason=homework');
  assert.ok(hw.priority > 200 && hw.priority < 300, 'below bedtime, above the time limit');
  assert.ok(hw.condition.excludedRequestDomains.includes('khanacademy.org') && hw.condition.excludedRequestDomains.includes('school.example'));
  assert.ok(!hw.condition.excludedRequestDomains.includes('bad.example'), 'a blocked site is not opened by homework time');
  assert.ok(by[RULE_ID.learnPath].priority > hw.priority, 'learning paths open in homework time');
  assert.ok(!buildRules({ learning }).some((x) => x.id === RULE_ID.paused || x.id === RULE_ID.homework));
  assert.equal(new Set(r.map((x) => x.id)).size, r.length, 'unique ids');

  const s = { learning, allow: ['school.example'], block: ['bad.example'], learningPaths: [{ host: 'bbc.co.uk', path: '/bitesize' }] };
  assert.equal(reasonFor('khanacademy.org', { ...s, paused: true }), 'paused', 'paused: learning sites too');
  assert.equal(reasonFor('127.0.0.1', { ...s, paused: true }), null);
  assert.equal(reasonFor('news.example', { ...s, homework: true }), 'homework');
  assert.equal(reasonFor('khanacademy.org', { ...s, homework: true }), null);
  assert.equal(reasonFor('school.example', { ...s, homework: true }), null, 'the parent\'s allowed sites open');
  assert.equal(reasonFor('bbc.co.uk', { ...s, homework: true }, 'https://www.bbc.co.uk/bitesize/maths'), null);
  assert.equal(reasonFor('bbc.co.uk', { ...s, homework: true }, 'https://www.bbc.co.uk/news'), 'homework');
  assert.equal(reasonFor('news.example', { ...s, homework: true, bedtime: true }), 'bedtime', 'bedtime comes first');
});
