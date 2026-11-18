// P2-251 (the owner, 18 Nov 2026): no helpline or crisis line on any child screen; the block page shows the
// emergency number for the country (the list starts at the browser's region); no PIN box is a password field.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { browserRegion, emergencyLine, countryOptions } from '../../ext/ui/emergency.js';

const read = (p) => readFileSync(new URL(`../../${p}`, import.meta.url), 'utf8');
const data = JSON.parse(read('shared/emergency.json'));
const N = data.numbers;
const dict = (lang) => JSON.parse(read(`ext/i18n/core.${lang}.json`));
const tFor = (lang) => { const en = dict('en'), own = dict(lang); return (k, v = {}) => String(own[k] ?? en[k] ?? k).replace(/\{(\w+)\}/g, (m, x) => v[x] ?? m); };
const t = tFor('en');

test('the data: over 200 countries, the main numbers, EU and EEA on 112, no retired codes', () => {
  assert.ok(Object.keys(N).length > 200, String(Object.keys(N).length));
  for (const [c, want] of Object.entries({ US: '911', CA: '911', MX: '911', GB: '999', GE: '112', RU: '112', UA: '112', DE: '112', FR: '112', AU: '000', NZ: '111', IN: '112', TR: '112' })) assert.equal(N[c], want, c);
  assert.deepEqual(N.JP, ['110', '119', '119']);
  for (const c of 'AT BE BG HR CY CZ DK EE FI FR DE GR HU IE IT LV LT LU MT NL PL PT RO SK SI ES SE IS LI NO'.split(' ')) assert.equal(N[c], '112', c);
  for (const c of ['DD', 'YD', 'YU', 'SU', 'FX', 'KP', 'RH']) assert.equal(N[c], undefined, c);
  // The review of 18 Nov 2026: Zimbabwe under its own code; national numbers where a mobile 112 is not confirmed.
  for (const [c, want] of Object.entries({ ZW: '999', CO: '123', PK: ['15', '1122', '16'], EG: ['122', '123', '180'], SG: ['999', '995', '995'], NP: ['100', '102', '101'], PE: ['105', '106', '116'], BO: ['110', '118', '119'], ET: ['991', '907', '939'], LB: ['112', '140', '175'] })) assert.deepEqual(N[c], want, c);
  // Every key is a country code a browser knows (a retired code would never be picked).
  const dn = new Intl.DisplayNames(['en'], { type: 'region' });
  for (const c of Object.keys(N)) assert.notEqual(dn.of(c), c, `${c} is not a current region code`);
  // Time zones point at countries in the list.
  assert.equal(data.zones['Asia/Tbilisi'], 'GE');
  assert.equal(data.zones['Asia/Calcutta'], 'IN', 'the older ICU name some browsers report');
  assert.equal(data.zones['Asia/Kolkata'], 'IN');
  for (const [z, c] of Object.entries(data.zones)) assert.ok(N[c], `${z} -> ${c}`);
  for (const v of Object.values(N)) for (const n of [].concat(v)) assert.match(n, /^[0-9]{0,6}$/);
});

test('the time zone comes first, then the browser region, then the region of the language', () => {
  assert.equal(browserRegion(['en-US', 'en'], N, 'Asia/Tbilisi', data.zones), 'GE', 'an en-US browser in Georgia');
  assert.equal(browserRegion(['en-US'], N, 'Etc/UTC', data.zones), 'US', 'no country for the zone: the language region');
  assert.equal(browserRegion(['en-US'], N, 'toString', data.zones), 'US', 'no prototype keys');
  assert.equal(browserRegion(['en-GB', 'en'], N), 'GB');
  assert.equal(browserRegion(['ka'], N), 'GE');
  assert.equal(browserRegion(['ru-RU'], N), 'RU');
  assert.equal(browserRegion(['en-US'], N), 'US');
  assert.equal(browserRegion(['fr', 'de-DE'], N), 'DE', 'a later tag with a region beats a bare first language');
  assert.equal(browserRegion(['xx-QQ'], N), '');
  assert.equal(browserRegion([], N), '');
});

test('the sentence: one number, or the services grouped by number; no helpline words', () => {
  assert.equal(emergencyLine(t, '112'), 'In an emergency, call 112 from a phone.');
  assert.equal(emergencyLine(t, N.JP), 'In an emergency, call from a phone: police 110, ambulance and fire 119.');
  assert.equal(emergencyLine(t, N.CN), 'In an emergency, call from a phone: police 110, ambulance 120, fire 119.');
  assert.equal(emergencyLine(t, undefined), 'In an emergency, call 112 from a phone.');
  assert.equal(emergencyLine(tFor('ka'), '112'), 'საგანგებო შემთხვევაში ტელეფონით დარეკე 112-ზე.');
  assert.equal(emergencyLine(tFor('ru'), N.JP), 'В экстренном случае позвони с телефона: полиция 110, скорая и пожарные 119.');
  for (const lang of ['en', 'ka', 'ru']) {
    const d = dict(lang);
    for (const k of ['sosOne', 'sosSplit', 'sosPolice', 'sosAmbulance', 'sosFire', 'sosAnd', 'sosCountry', 'sosChoose']) assert.ok(d[k], `${lang} ${k}`);
    // The child's screens (block page words); the parent settings may still say "Helplines stay open."
    const child = Object.entries(d).filter(([k]) => /^(r_|sos|bed|next|triedSite|backHome)/.test(k)).map(([, v]) => v).join(' ');
    assert.doesNotMatch(child, /helpline|suicid|talk to someone|ვინმეს დაელაპარაკო|с кем-нибудь поговорить/i, lang);
  }
});

test('the country list is named in the page language and sorted', () => {
  const en = countryOptions(N, 'en');
  assert.equal(en.length, Object.keys(N).length);
  assert.equal(en.find((o) => o.code === 'GE').name, 'Georgia');
  const ka = countryOptions(N, 'ka');
  assert.equal(ka.find((o) => o.code === 'GE').name, 'საქართველო');
  const names = en.map((o) => o.name);
  assert.deepEqual(names, [...names].sort(new Intl.Collator('en').compare));
});

test('the block page carries no helpline link; the emergency box sits under the words', () => {
  const html = read('ext/pages/blocked.html');
  assert.doesNotMatch(html, /findahelpline|helpLink/);
  assert.ok(html.indexOf('id="sos"') > html.indexOf('id="body"'));
  assert.doesNotMatch(read('ext/pages/blocked.js'), /findahelpline|\$\('help'\)/);
});

test('no PIN box is a password field: text drawn as dots, autocomplete off', () => {
  const pins = [
    ['ext/pages/blocked.html', ['pin']],
    ['ext/pages/options.html', ['lockPin', 'pin1', 'pin2']],
    ['agent/internal/api/web/parent.html', ['pinNew', 'pinAgain', 'pin', 'pinNow']],
  ];
  for (const [file, ids] of pins) {
    const html = read(file);
    assert.doesNotMatch(html, /type="password"/, file);
    for (const id of ids) {
      const tags = html.match(new RegExp(`<input id="${id}"[^>]*>`, 'g')) || [];
      assert.ok(tags.length > 0, `${file} #${id}`);
      for (const tag of tags) {
        assert.match(tag, /type="text"/, tag);
        assert.match(tag, /class="[^"]*kg-pin/, tag);
        assert.match(tag, /autocomplete="off"/, tag);
        assert.equal((tag.match(/autocomplete=/g) || []).length, 1, tag);
      }
    }
  }
  assert.match(read('ext/ui/kit.css'), /\.kg-pin\s*\{[^}]*-webkit-text-security:\s*disc/);
  assert.match(read('agent/internal/api/web/parent.css'), /input\.kg-pin\s*\{[^}]*-webkit-text-security:\s*disc/);
  const parent = read('agent/internal/api/web/parent.html');
  for (const a of ['setup', 'login', 'pin']) assert.match(parent, new RegExp(`action="/parent/${a}" autocomplete="off"`), a);
});
