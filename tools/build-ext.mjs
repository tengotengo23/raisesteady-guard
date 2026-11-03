#!/usr/bin/env node
// Build the RaiseSteady Guard extension.
//   node tools/build-ext.mjs            all builds + store zips
//   node tools/build-ext.mjs --dev      dev builds only (dist/chromium-dev, dist/firefox-dev)
//
// 1. copies shared/*.json into ext/data/ (never edit ext/data by hand)
// 2. creates placeholders for other desks' files only when absent (flag 'wx'; never overwrites)
// 3. makes the icons if absent; writes the display name (ui/brand.js APP_NAME) into _locales
// 4. checks the manifest's files, rulesets (valid JSON, unique ids, total <= 12 MB) and i18n keys
// 5. writes dist/chromium-dev, dist/firefox-dev (dev key + test hook), dist/chromium, dist/firefox
//    (store: no key, no test hook) and dist/raisesteady-guard-<version>-{chromium,firefox}.zip
import { readFileSync, writeFileSync, mkdirSync, readdirSync, statSync, existsSync, rmSync, copyFileSync } from 'node:fs';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { deflateSync } from 'node:zlib';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const EXT = join(ROOT, 'ext');
const SHARED = join(ROOT, 'shared');
const DIST = join(ROOT, 'dist');
const devOnly = process.argv.includes('--dev');
const log = (...a) => console.log('[build-ext]', ...a);
const fail = (msg) => { console.error('[build-ext] FAIL:', msg); process.exit(1); };
const readJSON = (p) => JSON.parse(readFileSync(p, 'utf8'));
const writeJSON = (p, v) => writeFileSync(p, JSON.stringify(v, null, 2) + '\n');

// ---- 1. shared data ----
mkdirSync(join(EXT, 'data'), { recursive: true });
const SKIP_SHARED = new Set(['ext-ids.json', 'unsupported-browsers.json']);
for (const f of readdirSync(SHARED)) {
  if (!f.endsWith('.json') || SKIP_SHARED.has(f) || / \d+\.json$/.test(f)) continue;
  const src = readFileSync(join(SHARED, f), 'utf8');
  JSON.parse(src); // must be valid
  const dst = join(EXT, 'data', f);
  if (!existsSync(dst) || readFileSync(dst, 'utf8') !== src) writeFileSync(dst, src);
}

// ---- 2. placeholders for desks B and D (only if absent) ----
const PLACEHOLDERS = {
  'background/videos.js': '// Placeholder the build writes when the file is missing.\nexport function initVideos() {}\n',
  'background/adblock.js': '// Placeholder the build writes when the file is missing.\nexport function initAdblock() {}\n',
  'content/focus-youtube.js': '// Placeholder the build writes when the file is missing.\n',
  'content/focus-social.js': '// Placeholder the build writes when the file is missing.\n',
  'content/cosmetic.js': '// Placeholder the build writes when the file is missing.\n',
  'content/yt-ads-main.js': '// Placeholder the build writes when the file is missing.\n',
  'ui/video-grid.js': '// Placeholder the build writes when the file is missing.\nexport async function renderVideoGrid() {}\n',
  'rules/cosmetic.css': '/* Placeholder the build writes when the file is missing. */\n',
  'rules/yt-ads.json': '[]\n',
  'rules/adult.json': '[]\n', 'rules/gambling.json': '[]\n', 'rules/bypass.json': '[]\n', 'rules/social.json': '[]\n', 'rules/ads.json': '[]\n',
};
for (const [rel, body] of Object.entries(PLACEHOLDERS)) {
  const p = join(EXT, rel);
  mkdirSync(dirname(p), { recursive: true });
  try { writeFileSync(p, body, { flag: 'wx' }); log('placeholder', rel); } catch (e) { if (e.code !== 'EEXIST') throw e; }
}

// ---- 3. icons and names ----
const APP_NAME = (/export const APP_NAME = '([^']+)'/.exec(readFileSync(join(EXT, 'ui/brand.js'), 'utf8')) || [])[1];
if (!APP_NAME) fail('APP_NAME not found in ext/ui/brand.js');
for (const lang of readdirSync(join(EXT, '_locales'))) {
  const p = join(EXT, '_locales', lang, 'messages.json');
  const m = readJSON(p);
  if (m.extName.message !== APP_NAME || m.extShortName.message !== APP_NAME) {
    m.extName.message = APP_NAME; m.extShortName.message = APP_NAME; writeJSON(p, m);
  }
  if ([...m.extDesc.message].length > 132) fail(`_locales/${lang} extDesc over 132 characters`);
  if ([...APP_NAME].length > 45) fail('APP_NAME over 45 characters');
}
mkdirSync(join(EXT, 'icons'), { recursive: true });
for (const size of [16, 32, 48, 128]) {
  const p = join(EXT, 'icons', `icon-${size}.png`);
  if (!existsSync(p)) { writeFileSync(p, iconPng(size)); log('icon', size); }
}

// ---- 4. checks ----
const manifest = readJSON(join(EXT, 'manifest.json'));
const files = [];
const want = (rel, why) => { if (!existsSync(join(EXT, rel))) fail(`missing ${rel} (${why})`); files.push(rel); };
want(manifest.background.service_worker, 'background');
for (const cs of manifest.content_scripts) { for (const f of cs.js || []) want(f, 'content script'); for (const f of cs.css || []) want(f, 'content css'); }
for (const f of [manifest.action.default_popup, manifest.options_ui.page, manifest.chrome_url_overrides.newtab, manifest.storage.managed_schema]) want(f, 'page');
for (const f of Object.values(manifest.icons)) want(f, 'icon');
let rulesBytes = 0;
for (const rr of manifest.declarative_net_request.rule_resources) {
  want(rr.path, 'ruleset');
  const raw = readFileSync(join(EXT, rr.path), 'utf8');
  rulesBytes += Buffer.byteLength(raw);
  let rules;
  try { rules = JSON.parse(raw); } catch (e) { fail(`${rr.path}: ${e.message}`); }
  if (!Array.isArray(rules)) fail(`${rr.path}: not an array`);
  const ids = new Set();
  for (const r of rules) { if (!Number.isInteger(r.id) || r.id < 1) fail(`${rr.path}: bad id`); if (ids.has(r.id)) fail(`${rr.path}: duplicate id ${r.id}`); ids.add(r.id); }
}
if (rulesBytes > 12 * 1024 * 1024) fail(`rulesets total ${(rulesBytes / 1048576).toFixed(1)} MB > 12 MB`);
const en = readJSON(join(EXT, 'i18n/core.en.json'));
for (const lang of ['ka', 'ru']) {
  const d = readJSON(join(EXT, `i18n/core.${lang}.json`));
  const miss = Object.keys(en).filter((k) => !(k in d));
  if (miss.length) fail(`i18n/core.${lang}.json missing: ${miss.join(', ')}`);
}
log(`checks ok: ${files.length} files, rulesets ${(rulesBytes / 1048576).toFixed(2)} MB`);

// ---- 5. outputs ----
const social = existsSync(join(EXT, 'data/social-domains.json')) ? readJSON(join(EXT, 'data/social-domains.json')).feeds || [] : [];

function copyTree(src, dst) {
  mkdirSync(dst, { recursive: true });
  for (const name of readdirSync(src)) {
    if (name === '.DS_Store' || / \d+(\.[^.]+)?$/.test(name)) continue; // iCloud " 2" copies stay out
    const s = join(src, name), d = join(dst, name);
    if (statSync(s).isDirectory()) copyTree(s, d); else copyFileSync(s, d);
  }
}

function targetManifest(target, dev) {
  const m = structuredClone(manifest);
  if (!dev) delete m.key;
  if (social.length) {
    const cs = m.content_scripts.find((c) => (c.js || []).includes('content/focus-social.js'));
    if (cs) cs.matches = [...new Set(social.map((f) => `*://*.${f.domain}/*`))];
  }
  if (target === 'firefox') {
    delete m.key;
    delete m.minimum_chrome_version;
    delete m.storage; // Firefox reads managed storage from policies.json 3rdparty; no schema key
    m.background = { scripts: [manifest.background.service_worker], type: 'module' };
    m.browser_specific_settings = {
      gecko: {
        id: readJSON(join(SHARED, 'ext-ids.json')).firefox.id,
        strict_min_version: '140.0',
        // Time and domain counts go to the RaiseSteady Guard agent on this computer (127.0.0.1) for the parent.
        data_collection_permissions: { required: ['browsingActivity'] },
      },
      gecko_android: { strict_min_version: '142.0' },
    };
  }
  return m;
}

function build(target, dev) {
  const out = join(DIST, `${target}${dev ? '-dev' : ''}`);
  rmSync(out, { recursive: true, force: true });
  copyTree(EXT, out);
  // Licences and attributions of the lists and datasets ship inside every build and zip.
  const notices = join(ROOT, 'THIRD-PARTY-NOTICES.txt');
  if (!existsSync(notices)) fail('missing THIRD-PARTY-NOTICES.txt');
  copyFileSync(notices, join(out, 'THIRD-PARTY-NOTICES.txt'));
  writeJSON(join(out, 'manifest.json'), targetManifest(target, dev));
  writeFileSync(join(out, 'background/build-info.js'), `// Written by tools/build-ext.mjs\nexport const BUILD = ${JSON.stringify({ dev, target, builtAt: new Date().toISOString() })};\n`);
  log('wrote', relative(ROOT, out));
  return out;
}

mkdirSync(DIST, { recursive: true });
build('chromium', true);
build('firefox', true);
if (!devOnly) {
  for (const target of ['chromium', 'firefox']) {
    const out = build(target, false);
    const zip = join(DIST, `raisesteady-guard-${manifest.version}-${target}.zip`);
    rmSync(zip, { force: true });
    execFileSync('zip', ['-r', '-X', '-q', zip, '.'], { cwd: out });
    log('zip', relative(ROOT, zip), `${(statSync(zip).size / 1048576).toFixed(2)} MB`);
  }
}

// ---- icon: navy rounded square, yellow sun, mint hill (tokens.css colours), 4x supersampled ----
function iconPng(S) {
  const navy = [0x28, 0x28, 0x58], yellow = [0xfe, 0xdb, 0x71], mint = [0x75, 0xc5, 0xa8];
  const N = 4, px = Buffer.alloc(S * S * 4);
  const rr = 0.22;
  const inSquare = (x, y) => {
    const cx = Math.min(Math.max(x, rr), 1 - rr), cy = Math.min(Math.max(y, rr), 1 - rr);
    return (x - cx) ** 2 + (y - cy) ** 2 <= rr * rr;
  };
  for (let y = 0; y < S; y++) for (let x = 0; x < S; x++) {
    const acc = [0, 0, 0, 0];
    for (let sy = 0; sy < N; sy++) for (let sx = 0; sx < N; sx++) {
      const u = (x + (sx + 0.5) / N) / S, v = (y + (sy + 0.5) / N) / S;
      if (!inSquare(u, v)) continue;
      let c = navy;
      if ((u - 0.5) ** 2 + (v - 1.18) ** 2 <= 0.56 ** 2) c = mint;
      else if ((u - 0.5) ** 2 + (v - 0.4) ** 2 <= 0.19 ** 2) c = yellow;
      acc[0] += c[0]; acc[1] += c[1]; acc[2] += c[2]; acc[3] += 255;
    }
    const n = N * N, a = acc[3] / n, i = (y * S + x) * 4;
    px[i] = a ? Math.round(acc[0] / (acc[3] / 255)) : 0; px[i + 1] = a ? Math.round(acc[1] / (acc[3] / 255)) : 0;
    px[i + 2] = a ? Math.round(acc[2] / (acc[3] / 255)) : 0; px[i + 3] = Math.round(a);
  }
  const raw = Buffer.alloc((S * 4 + 1) * S);
  for (let y = 0; y < S; y++) { raw[y * (S * 4 + 1)] = 0; px.copy(raw, y * (S * 4 + 1) + 1, y * S * 4, (y + 1) * S * 4); }
  const crcT = Array.from({ length: 256 }, (_, n) => { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; return c >>> 0; });
  const crc = (b) => { let c = 0xffffffff; for (const x of b) c = crcT[(c ^ x) & 0xff] ^ (c >>> 8); return (c ^ 0xffffffff) >>> 0; };
  const chunk = (type, data) => {
    const len = Buffer.alloc(4); len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type), data]);
    const c = Buffer.alloc(4); c.writeUInt32BE(crc(td));
    return Buffer.concat([len, td, c]);
  };
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(S, 0); ihdr.writeUInt32BE(S, 4); ihdr[8] = 8; ihdr[9] = 6;
  return Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0))]);
}
