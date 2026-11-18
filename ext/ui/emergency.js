// Emergency numbers on the block page (P2-251): "In an emergency, call 112 from a phone." and a country
// list that starts at the browser's region. Data: data/emergency.json (tools/update-emergency.mjs).

const STORE_KEY = 'kg-sos-country';

/** The browser's region: the country of the time zone first (many browsers say en-US wherever they are), then
 *  the first language tag that names one ("en-GB" -> GB), else the likely region of a language ("ka" -> GE).
 *  '' when nothing fits the data. */
export function browserRegion(langs, numbers, tz = '', zones = {}) {
  const z = tz && Object.hasOwn(zones, tz) ? zones[tz] : '';
  if (z && numbers[z]) return z;
  const list = (Array.isArray(langs) ? langs : [langs]).filter(Boolean).map(String);
  for (const tag of list) {
    try { const r = new Intl.Locale(tag).region; if (r && numbers[r]) return r; } catch { /* bad tag */ }
  }
  for (const tag of list) {
    try { const r = new Intl.Locale(tag).maximize().region; if (r && numbers[r]) return r; } catch { /* bad tag */ }
  }
  return '';
}

/** The sentence for one country: one number, or police, ambulance and fire (same numbers joined). */
export function emergencyLine(t, entry) {
  if (typeof entry === 'string' && entry) return t('sosOne', { num: entry });
  if (!Array.isArray(entry)) return t('sosOne', { num: '112' });
  const groups = [];
  ['sosPolice', 'sosAmbulance', 'sosFire'].forEach((key, i) => {
    const num = entry[i];
    if (!num) return;
    const g = groups.find((x) => x.num === num);
    if (g) g.keys.push(key); else groups.push({ num, keys: [key] });
  });
  const parts = groups.map((g) => `${g.keys.map((k) => t(k)).join(t('sosAnd'))} ${g.num}`);
  return t('sosSplit', { list: parts.join(', ') });
}

/** Country options sorted by their name in the page language: [{ code, name }]. */
export function countryOptions(numbers, lang) {
  let dn = null;
  try { dn = new Intl.DisplayNames([lang, 'en'], { type: 'region' }); } catch { dn = null; }
  const name = (c) => { try { return (dn && dn.of(c)) || c; } catch { return c; } };
  let coll;
  try { coll = new Intl.Collator(lang); } catch { coll = new Intl.Collator('en'); }
  return Object.keys(numbers).map((code) => ({ code, name: name(code) })).sort((a, b) => coll.compare(a.name, b.name));
}

function saved() { try { return localStorage.getItem(STORE_KEY) || ''; } catch { return ''; } }
function save(c) { try { localStorage.setItem(STORE_KEY, c); } catch { /* storage off */ } }

/** Fill #sosLine and #sosCountry inside box and show it. */
export async function setupEmergency(box, t, { api = globalThis.browser ?? globalThis.chrome, langs = navigator.languages || [navigator.language] } = {}) {
  let data = {};
  try { data = (await (await fetch(api.runtime.getURL('data/emergency.json'))).json()) || {}; } catch { data = {}; }
  const numbers = data.numbers || {};
  let tz = '';
  try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch { tz = ''; }
  const line = box.querySelector('#sosLine'), sel = box.querySelector('#sosCountry');
  const pick = (c) => { line.textContent = emergencyLine(t, numbers[c]); };
  const start = numbers[saved()] ? saved() : browserRegion(langs, numbers, tz, data.zones || {});
  const opts = countryOptions(numbers, t.lang);
  const doc = box.ownerDocument;
  if (!start) {
    const o = doc.createElement('option');
    o.value = ''; o.textContent = t('sosChoose');
    sel.append(o);
  }
  for (const { code, name } of opts) {
    const o = doc.createElement('option');
    o.value = code; o.textContent = name;
    sel.append(o);
  }
  sel.value = start;
  pick(start);
  sel.addEventListener('change', () => { pick(sel.value); if (sel.value) save(sel.value); });
  box.hidden = false;
}
