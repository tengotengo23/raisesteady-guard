#!/usr/bin/env node
// Emergency numbers for the block page (P2-251): shared/emergency.json from Wikipedia's
// "List of emergency telephone numbers" (raw wikitext, CC BY-SA 4.0).
//
//   node tools/update-emergency.mjs [--from file.wiki]
//
// Each country is one number ("112") when one number reaches every service (the same number
// in the police, ambulance and fire columns, or the note "Mobile phones – N" / "General
// emergencies – N"), else [police, ambulance, fire]. EU and EEA countries get 112 (EU law:
// 112 reaches every emergency service in every member state). Retired codes are dropped;
// places without an ISO 3166 code are left out.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const OUT = path.join(ROOT, 'shared', 'emergency.json');
const SRC = 'https://en.wikipedia.org/w/index.php?title=List_of_emergency_telephone_numbers&action=raw';

const EU_EEA = 'AT BE BG HR CY CZ DK EE FI FR DE GR HU IE IT LV LT LU MT NL PL PT RO SK SI ES SE IS LI NO'.split(' ');
// Rows the table writes in a way the parser cannot read (checked by hand against the row, 18 Nov 2026).
// Checked by hand, 18 Nov 2026 (P2-251 review): where the page's single number is not confirmed to work across
// the country, the national service numbers. Peru's 911 is in trial in Lima and Callao only (Infobae, June 2026);
// Bolivia's and Ethiopia's 911 are not confirmed; Lebanon's police (ISF) number is 112; Egypt's police 122;
// Pakistan's ambulance Rescue 1122 (the page's first is Edhi's 115).
const FIX = { PH: '911', PE: ['105', '106', '116'], BO: ['110', '118', '119'], ET: ['991', '907', '939'], LB: ['112', '140', '175'], EG: ['122', '123', '180'], PK: ['15', '1122', '16'] };
// Time zones to countries (tzdata zone.tab, public domain), so an en-US browser in Georgia starts at Georgia.
// Browsers report ICU's zone names, some of them older than tzdata's: those are added by hand.
const ZONE_TAB = '/usr/share/zoneinfo/zone.tab';
const OLD_ZONES = { 'Asia/Calcutta': 'IN', 'Asia/Katmandu': 'NP', 'Asia/Saigon': 'VN', 'Asia/Rangoon': 'MM', 'Europe/Kiev': 'UA', 'Asia/Thimbu': 'BT', 'Atlantic/Faeroe': 'FO', 'America/Godthab': 'GL', 'Asia/Ulan_Bator': 'MN', 'Asia/Dacca': 'BD', 'Asia/Macao': 'MO', 'Asia/Ujung_Pandang': 'ID', 'America/Buenos_Aires': 'AR', 'America/Indianapolis': 'US', 'America/Louisville': 'US', 'Pacific/Truk': 'FM', 'Pacific/Ponape': 'FM', 'Pacific/Enderbury': 'KI', 'Africa/Asmera': 'ER', 'Asia/Ashkhabad': 'TM', 'Asia/Chungking': 'CN', 'Asia/Tel_Aviv': 'IL', 'Europe/Belfast': 'GB', 'America/Montreal': 'CA', 'Australia/Canberra': 'AU', 'Asia/Istanbul': 'TR', 'Europe/Nicosia': 'CY' };
export function zones(tab, numbers) {
  const out = {};
  for (const line of tab.split('\n')) {
    const m = line.match(/^([A-Z]{2})\t\S+\t(\S+)/);
    if (m && numbers[m[1]]) out[m[2]] = m[1];
  }
  for (const [z, c] of Object.entries(OLD_ZONES)) if (numbers[c] && !out[z]) out[z] = c;
  return Object.fromEntries(Object.entries(out).sort());
}
const RETIRED = new Set('DD YD DY HV FX YU ZR BU TP CS SU NT AN QO EU UN EZ XA XB ZZ TA VD NH RH'.split(' '));
// Left out on purpose: North Korea ("local numbers only").
const SKIP = new Set(['KP']);
const norm = (n) => n.toLowerCase().replace(/&/g, 'and').replace(/^st\. /, 'saint ').replace(/ sar china$/, '').replace(/ \(.*\)$/, '').replace(/^türkiye$/, 'turkey').replace(/^palestinian territories$/, 'palestine').trim();
const ALIAS = {
  'united states of america': 'US', 'republic of china': 'TW', 'people\'s republic of china': 'CN',
  'republic of congo': 'CG', 'democratic republic of congo': 'CD', 'democratic republic of the congo': 'CD',
  'saint helena': 'SH', 'curacao': 'CW', 'turks and caicos': 'TC', 'saint pierre and miquelon': 'PM',
  'south georgia and the south sandwich islands': 'GS', 'czech republic': 'CZ', 'ivory coast': 'CI',
  'east timor': 'TL', 'the gambia': 'GM', 'the bahamas': 'BS', 'federated states of micronesia': 'FM',
  'vatican city': 'VA', 'state of palestine': 'PS', 'republic of ireland': 'IE', 'cape verde': 'CV',
  'sao tome and principe': 'ST', 'united states virgin islands': 'VI', 'u.s. virgin islands': 'VI',
  'kosovo': 'XK', 'saint vincent and the grenadines': 'VC', 'turkey': 'TR', 'palestine': 'PS', 'macau': 'MO', 'myanmar': 'MM', 'collectivity of saint martin': 'MF', 'caribbean netherlands': 'BQ', 'åland': 'AX',
};

function codesByName() {
  const dn = new Intl.DisplayNames(['en'], { type: 'region' });
  const A = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', m = {};
  for (const x of A) for (const y of A) {
    const c = x + y;
    if (RETIRED.has(c)) continue;
    let n; try { n = dn.of(c); } catch { continue; }
    if (n && n !== c && !m[norm(n)]) m[norm(n)] = c;
  }
  return m;
}

const NUM = /'''\s*(?:\[\[[^\]|]*\|)?\s*([0-9][0-9 ]*[0-9]|[0-9])\s*(?:\]\])?\s*(?:or\s*[0-9 ]+)?;?\s*'''/g;
const nums = (s) => [...s.matchAll(NUM)].map((m) => m[1].replace(/ /g, ''));

export function parse(wiki) {
  let t = wiki.split('== See also ==')[0];
  t = t.replace(/\(\+[0-9]+\)\s*/g, '').replace(/<ref[^>]*\/>/g, '').replace(/<ref[^>]*>[\s\S]*?<\/ref>/g, '').replace(/\{\{efn[\s\S]*?\}\}/g, '');
  const byName = codesByName(), out = {}, missing = [];
  for (const row of t.split('\n|-')) {
    const m = row.match(/\{\{[Ff]lag[a-z]*\|([^}|]+)((?:\|[^}]*)?)\}\}/);
    if (!m) continue;
    const nm = (m[2] || '').match(/name=([^}|]+)/);
    const name = (nm ? nm[1] : m[1]).trim();
    const cells = [];
    for (const l of row.slice(m.index + m[0].length).split('\n')) {
      if (!l.startsWith('|') || l.startsWith('|}')) continue;
      for (const part of l.slice(1).split('||')) cells.push(part);
    }
    const cols = [];
    let note = '';
    for (const c of cells) {
      let span = 1, body = c;
      const head = c.split('|')[0];
      if (c.includes('|') && /(colspan|style|align|rowspan)/.test(head)) {
        body = c.slice(head.length + 1);
        const s = head.match(/colspan\s*=\s*"?(\d)/);
        span = s ? Number(s[1]) : 1;
      }
      if (cols.length >= 3) { note += ' ' + body; continue; }
      // "112 or 166" inside one bold run: both numbers
      const found = [...body.matchAll(/'''([^']+)'''/g)].flatMap((x) => (x[1].match(/[0-9][0-9 ]*[0-9]|[0-9]/g) || []).map((n) => n.replace(/ /g, '')))
        .filter((n) => !/^\+/.test(n));
      for (let i = 0; i < span; i++) cols.push(found);
    }
    if (cols.length < 3) continue;
    const code = ALIAS[name.toLowerCase()] || byName[norm(name)];
    if (!code) { missing.push(name); continue; }
    if (out[code] || SKIP.has(code)) continue;
    const [p, a, f] = cols;
    const common = p.filter((n) => a.includes(n) && f.includes(n));
    let one = common[0] || null;
    if (!one) {
      // A number for every emergency. Not "Mobile phones – 112": where the page also lists national numbers, the
      // mobile 112 is not confirmed to reach help (Pakistan, Nepal, Egypt: review of 18 Nov 2026).
      const g = note.match(/(?:[Gg]eneral emergenc(?:y|ies)|[Aa]ll emergencies|[Aa]ny emergency)\s*[–-]\s*'''\s*([0-9 ]+?)\s*[;.,]?\s*'''/);
      if (g) one = g[1].replace(/ /g, '');
    }
    if (one) out[code] = one;
    else if (p[0] || a[0] || f[0]) out[code] = [p[0] || '', a[0] || '', f[0] || ''];
  }
  for (const c of EU_EEA) out[c] = '112';
  Object.assign(out, FIX);
  return { numbers: Object.fromEntries(Object.entries(out).sort()), missing };
}

async function main() {
  const i = process.argv.indexOf('--from');
  const wiki = i > 0 ? fs.readFileSync(process.argv[i + 1], 'utf8')
    : await (await fetch(SRC, { headers: { 'user-agent': 'raisesteady-guard-build/1.0 (https://github.com/tengotengo23/raisesteady-guard)' } })).text();
  const { numbers, missing } = parse(wiki);
  const n = Object.keys(numbers).length;
  if (n < 200) throw new Error(`only ${n} countries parsed: the table changed, check the parser`);
  for (const [c, want] of Object.entries({ US: '911', CA: '911', GB: '999', GE: '112', RU: '112', AU: '000', NZ: '111', DE: '112', IN: '112', ZW: '999', CO: '123' })) {
    if (numbers[c] !== want) throw new Error(`${c} parsed as ${JSON.stringify(numbers[c])}, expected ${want}`);
  }
  const doc = {
    source: 'Wikipedia, List of emergency telephone numbers (CC BY-SA 4.0); EU and EEA: 112; some checked by hand (tools/update-emergency.mjs). Zones: tzdata zone.tab (public domain)',
    fetched: new Date().toISOString().slice(0, 10),
    note: 'One string: one number for every service. Array: police, ambulance, fire.',
    numbers,
    zones: zones(fs.readFileSync(ZONE_TAB, 'utf8'), numbers),
  };
  if (doc.zones['Asia/Tbilisi'] !== 'GE' || doc.zones['America/New_York'] !== 'US' || Object.keys(doc.zones).length < 300) throw new Error('zone.tab did not parse');
  fs.writeFileSync(OUT, JSON.stringify(doc, null, 0).replace(/,"numbers":/, ',\n"numbers":').replace(/,"zones":/, ',\n"zones":') + '\n');
  console.log(`${n} countries -> ${path.relative(ROOT, OUT)}; one number: ${Object.values(numbers).filter((v) => typeof v === 'string').length}; left out (no ISO code): ${missing.join(', ') || 'none'}`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) main().catch((e) => { console.error('✗', e.message); process.exit(1); });
