#!/usr/bin/env node
// Build ext/rules/cosmetic.css from EasyList element-hiding rules.
//   node tools/build-cosmetic.mjs              build from the pinned files in .cache (download if absent)
//   node tools/build-cosmetic.mjs --validate   also check every selector in headless Chrome and drop the ones it rejects
//
// Source: EasyList (https://easylist.to/), GPLv3 or CC BY-SA 3.0, used under CC BY-SA 3.0 with attribution.
// Pinned to one commit of github.com/easylist/easylist so the output is reproducible.
// Our own code (not derived from any GPL converter).
//
// Output shape (CSS nesting; Chrome 120+, Firefox 117+). One outer rule per kind, so the gate is written once:
//   html:not([data-kg-ads="off"]):not(<generichide sites>) {
//     .a, #b, ... { display: none !important; }              generic, in chunks (a bad selector only drops its chunk)
//     &:not(<sites with an exception>) .sel { ... }           generic with exceptions
//   }
//   html:not([data-kg-ads="off"]) { &:is([data-kg-h~="site.com"]) { .sel, ... { ... } } }   site-specific, biggest sites first
// content/cosmetic.js writes data-kg-h (the host and its parent domains) and data-kg-ads="off" when the blocker is off.
import { readFileSync, writeFileSync, existsSync, mkdirSync, statSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import { loadPuppeteer } from './puppeteer.mjs';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const CACHE = join(ROOT, '.cache');
export const EASYLIST_COMMIT = '7be4364afb7d5b776b2604240a7e9f071f1c1ba4'; // 2026-11-03T10:37:42Z
export const TRANCO_LIST = 'GQ38K'; // Tranco top sites, 2026-10-04..2026-11-02 (only used to choose which sites get specific rules)
const FILES = ['easylist_general_hide', 'easylist_specific_hide', 'easylist_allowlist_general_hide', 'easylist_allowlist'];
const MAX_BYTES = 400 * 1024;
const CHUNK = 120;
const GATE = 'html:not([data-kg-ads="off"])';
const HIDE = '{ display: none !important; }';

// Extended (non-CSS) syntax from ABP / uBO / AdGuard: never valid in a stylesheet.
const EXTENDED = /:-abp-|:has-text\(|:contains\(|:xpath\(|:upward\(|:remove\(|:style\(|:matches-|:min-text-length\(|:watch-attr\(|:others\(|:if\(|:if-not\(|:nth-ancestor\(|:remove-attr|:remove-class|:-ext-/;

/** A selector we can put in a stylesheet without risk: no rule breaks, comments or extended syntax. */
export function safeSelector(sel) {
  if (typeof sel !== 'string') return false;
  const s = sel.trim();
  if (!s || s.length > 500) return false;
  if (/[{}<\n\r]|\/\*|\*\/|\\$/.test(s)) return false;
  if (EXTENDED.test(s)) return false;
  if (/^(html|:root)\b/i.test(s)) return false; // nesting puts our html gate in front; these could never match
  let depth = 0, quote = null; // balanced brackets and quotes
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c === '\\') { i++; continue; }
    if (quote) { if (c === quote) quote = null; continue; }
    if (c === '"' || c === "'") quote = c;
    else if (c === '(' || c === '[') depth++;
    else if (c === ')' || c === ']') { depth--; if (depth < 0) return false; }
  }
  return depth === 0 && !quote;
}

const DOMAIN_RE = /^[a-z0-9.-]+$/;
const cleanDomain = (d) => String(d || '').trim().toLowerCase().replace(/^\.+|\.+$/g, '');

/** Parse element-hiding lines. Returns { generic: Set, exceptions: Map(sel -> Set(domains)), specific: [{ sel, include:[], exclude:[] }] }. */
export function parseHiding(text) {
  const generic = new Set();
  const exceptions = new Map();
  const specific = [];
  let skipped = 0;
  for (const raw of String(text).split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line[0] === '!' || line[0] === '[') continue;
    const m = /^([^#]*)(##|#@#|#\?#|#\$#|#@\$#|#@\?#|#%#|#@%#)(.+)$/.exec(line);
    if (!m) continue;
    const [, doms, sep, sel] = m;
    if (sep !== '##' && sep !== '#@#') { skipped++; continue; }
    if (!safeSelector(sel)) { skipped++; continue; }
    const include = [], exclude = [];
    let bad = false;
    for (const d0 of doms.split(',').map((x) => x.trim()).filter(Boolean)) {
      const neg = d0.startsWith('~');
      const d = cleanDomain(neg ? d0.slice(1) : d0);
      if (!DOMAIN_RE.test(d) || d.endsWith('.*') || d.includes('*')) { bad = true; break; }
      (neg ? exclude : include).push(d);
    }
    if (bad) { skipped++; continue; }
    const s = sel.trim();
    if (sep === '#@#') {
      for (const d of include) { if (!exceptions.has(s)) exceptions.set(s, new Set()); exceptions.get(s).add(d); }
      continue;
    }
    if (!include.length && !exclude.length) generic.add(s);
    else if (!include.length) { // generic except some sites
      generic.add(s);
      for (const d of exclude) { if (!exceptions.has(s)) exceptions.set(s, new Set()); exceptions.get(s).add(d); }
    } else specific.push({ sel: s, include, exclude });
  }
  return { generic, exceptions, specific, skipped };
}

/** Sites where EasyList switches generic hiding off ($generichide / $elemhide on the whole site). */
export function parseGenericHide(text) {
  const out = new Set();
  for (const raw of String(text).split(/\r?\n/)) {
    const line = raw.trim();
    if (!line.startsWith('@@') || !/\$(.*,)?(generichide|elemhide|ghide|ehide)(,|$)/.test(line)) continue;
    const opts = line.slice(line.lastIndexOf('$') + 1);
    if (/(^|,)domain=/.test(opts)) continue; // scoped to embedding sites: skip, cannot express
    const m = /^@@(?:\|\||:\/\/|\|https?:\/\/)([a-z0-9.-]+)(?:[\^/]|\$)/i.exec(line);
    if (!m) continue;
    const host = cleanDomain(m[1]);
    // only whole-site rules: nothing after the host but ^ or /
    const rest = line.slice(m[0].length - 1, line.lastIndexOf('$'));
    if (rest && !/^[\^/]?$/.test(rest)) continue;
    if (DOMAIN_RE.test(host) && host.includes('.')) out.add(host);
  }
  return out;
}

const attrIs = (d) => `[data-kg-h~="${d}"]`;
const notSites = (set) => (set && set.size ? `:not(${[...set].sort().map(attrIs).join(',')})` : '');

/** Registrable-ish base: the last two labels, or three for common second-level public suffixes. */
export function baseDomain(host) {
  const p = cleanDomain(host).split('.');
  if (p.length <= 2) return p.join('.');
  const sl = p[p.length - 2];
  const two = ['co', 'com', 'net', 'org', 'gov', 'edu', 'ac', 'or', 'ne', 'go'].includes(sl) && p[p.length - 1].length === 2;
  return p.slice(two ? -3 : -2).join('.');
}

/**
 * Build the stylesheet. `topSites`: Map(domain -> rank) of sites that get their specific rules;
 * `alwaysSites`: specific rules always kept for these (YouTube). `invalid`: selectors Chrome rejected.
 */
export function buildCss({ hiding, genericHide, topSites = new Map(), alwaysSites = [], maxBytes = MAX_BYTES, invalid = new Set(), header = '' }) {
  const ok = (s) => !invalid.has(s);
  const plain = [], excepted = [];
  for (const s of hiding.generic) {
    if (!ok(s)) continue;
    if (hiding.exceptions.has(s)) excepted.push(s); else plain.push(s);
  }
  plain.sort();
  const parts = [];
  let chunks = 0;
  for (let i = 0; i < plain.length; i += CHUNK) {
    parts.push(`  ${plain.slice(i, i + CHUNK).join(',\n  ')} ${HIDE}`);
    chunks++;
  }
  for (const s of excepted.sort()) {
    parts.push(`  &${notSites(hiding.exceptions.get(s))} ${s} ${HIDE}`);
    chunks++;
  }
  parts.unshift(`${GATE}${notSites(genericHide)} {`);
  parts.push('}');
  const genericBytes = Buffer.byteLength(parts.join('\n'));

  // Site-specific: group by the include/exclude sets, ranked by the best site rank in the group.
  const always = new Set(alwaysSites);
  const groups = new Map();
  for (const r of hiding.specific) {
    if (!ok(r.sel)) continue;
    const exc = hiding.exceptions.get(r.sel);
    const include = r.include.filter((d) => !(exc && exc.has(d)));
    const keep = include.filter((d) => always.has(baseDomain(d)) || topSites.has(d) || topSites.has(baseDomain(d)));
    if (!keep.length) continue;
    const key = `${keep.sort().join(',')}|${r.exclude.sort().join(',')}`;
    if (!groups.has(key)) {
      const rank = Math.min(...keep.map((d) => (always.has(baseDomain(d)) ? 0 : topSites.get(d) ?? topSites.get(baseDomain(d)) ?? 1e9)));
      groups.set(key, { include: keep, exclude: r.exclude, sels: [], rank });
    }
    groups.get(key).sels.push(r.sel);
  }
  const ordered = [...groups.values()].sort((a, b) => a.rank - b.rank || a.include[0].localeCompare(b.include[0]));
  let bytes = Buffer.byteLength(header) + genericBytes;
  const siteParts = [];
  const sites = new Set();
  let specificRules = 0, droppedGroups = 0;
  for (const g of ordered) {
    const sel = [...new Set(g.sels)].sort();
    const blocks = [];
    for (let i = 0; i < sel.length; i += CHUNK) {
      const ex = g.exclude.length ? `:not(${g.exclude.map(attrIs).join(',')})` : '';
      blocks.push(`  &:is(${g.include.map(attrIs).join(',')})${ex} {\n    ${sel.slice(i, i + CHUNK).join(',\n    ')} ${HIDE}\n  }`);
    }
    const text = blocks.join('\n');
    const size = Buffer.byteLength(text) + 1;
    if (bytes + size > maxBytes && g.rank > 0) { droppedGroups++; continue; }
    bytes += size;
    siteParts.push(text);
    chunks += blocks.length;
    specificRules += sel.length;
    for (const d of g.include) sites.add(d);
  }
  const css = `${header}${parts.join('\n')}\n${GATE} {\n${siteParts.join('\n')}\n}\n`;
  return {
    css, chunks,
    stats: { generic: plain.length + excepted.length, exceptions: excepted.length, genericHideSites: genericHide.size, specificRules, specificSites: sites.size, droppedGroups, bytes: Buffer.byteLength(css) },
  };
}

// ---- runtime ----
function fetchPinned(name) {
  const p = join(CACHE, `b-${name}.txt`);
  if (!existsSync(p)) {
    mkdirSync(CACHE, { recursive: true });
    const url = `https://raw.githubusercontent.com/easylist/easylist/${EASYLIST_COMMIT}/easylist/${name}.txt`;
    execFileSync('curl', ['-sSfL', '-o', p, url]);
  }
  return readFileSync(p, 'utf8');
}

function loadTopSites(n) {
  const p = join(CACHE, `b-tranco-${TRANCO_LIST}-top1m.csv`);
  if (!existsSync(p)) execFileSync('curl', ['-sSfL', '-o', p, `https://tranco-list.eu/download/${TRANCO_LIST}/1000000`]);
  const top = new Map();
  const lines = readFileSync(p, 'utf8').split('\n');
  for (let i = 0; i < Math.min(n, lines.length); i++) {
    const [rank, dom] = lines[i].trim().split(',');
    if (dom) top.set(dom.toLowerCase(), Number(rank));
  }
  return top;
}

async function validateInChrome(selectors) {
  const REPO = join(ROOT, '..');
  const puppeteer = loadPuppeteer();
  const browser = await puppeteer.launch({ headless: true, pipe: true, args: ['--no-first-run'] });
  try {
    const page = await browser.newPage();
    await page.setContent('<!doctype html><title>v</title>');
    return new Set(await page.evaluate((list) => list.filter((s) => {
      try { document.createDocumentFragment().querySelector(s); } catch { return true; }
      const sh = new CSSStyleSheet();
      try { sh.replaceSync(`html { ${s} { display: none } }`); } catch { return true; }
      return !(sh.cssRules[0] && sh.cssRules[0].cssRules && sh.cssRules[0].cssRules.length === 1);
    }), [...selectors]));
  } finally { await browser.close(); }
}

async function main() {
  const t0 = Date.now();
  const general = fetchPinned('easylist_general_hide');
  const specific = fetchPinned('easylist_specific_hide');
  const allowHide = fetchPinned('easylist_allowlist_general_hide');
  const allow = fetchPinned('easylist_allowlist');
  const hiding = parseHiding([general, specific, allowHide].join('\n'));
  const genericHide = parseGenericHide(allow);
  const topSites = loadTopSites(Number(process.env.KG_TOP_SITES || 3000));
  const invPath = join(CACHE, 'b-invalid-selectors.json');
  let invalid = existsSync(invPath) ? new Set(JSON.parse(readFileSync(invPath, 'utf8'))) : new Set();
  if (process.argv.includes('--validate')) {
    const all = new Set([...hiding.generic, ...hiding.specific.map((r) => r.sel)]);
    invalid = await validateInChrome(all);
    writeFileSync(invPath, JSON.stringify([...invalid].sort(), null, 1));
    console.log(`[build-cosmetic] validated ${all.size} selectors in Chrome: ${invalid.size} rejected`);
  }
  const date = /Last modified: ([^\n]+)/.exec(general)?.[1] || 'see commit';
  const header = `/* RaiseSteady Guard cosmetic ad hiding, built by tools/build-cosmetic.mjs. Do not edit by hand.\n` +
    ` * Element-hiding rules from EasyList, (c) The EasyList authors (https://easylist.to/), used under CC BY-SA 3.0\n` +
    ` * (https://creativecommons.org/licenses/by-sa/3.0/); this derived file is shared under the same licence.\n` +
    ` * Source: github.com/easylist/easylist commit ${EASYLIST_COMMIT} (${date}). Site choice: Tranco list ${TRANCO_LIST}.\n` +
    ` * Off switch: html[data-kg-ads="off"] (set by content/cosmetic.js). */\n`;
  const out = buildCss({ hiding, genericHide, topSites, alwaysSites: ['youtube.com'], invalid, header });
  const dst = join(ROOT, 'ext', 'rules', 'cosmetic.css');
  writeFileSync(dst, out.css);
  writeFileSync(join(CACHE, 'b-cosmetic-report.json'), JSON.stringify({ builtAt: new Date().toISOString(), commit: EASYLIST_COMMIT, chunks: out.chunks, skippedExtended: hiding.skipped, invalid: invalid.size, ...out.stats }, null, 2));
  console.log(`[build-cosmetic] ${out.stats.generic} generic (${out.stats.exceptions} with exceptions), ${out.stats.genericHideSites} generichide sites, ${out.stats.specificRules} site rules on ${out.stats.specificSites} sites, ${out.chunks} blocks, skipped ${hiding.skipped} extended, ${invalid.size} invalid; ${(statSync(dst).size / 1024).toFixed(1)} KB in ${Date.now() - t0} ms`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  main().catch((e) => { console.error('[build-cosmetic] FAIL', e); process.exit(1); });
}
