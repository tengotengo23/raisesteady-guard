#!/usr/bin/env node
// Feed-free mode on scroll sites, live (band 13-15, logged out).
//   node tools/build-ext.mjs --dev && node tests/e2e/b-social.mjs
// Logged out, most sites show a sign-in wall instead of a feed; this suite records what each site showed,
// so the return can say honestly which sites were really tested.
// Hard checks: Reddit (its home feed is public): feed hidden, our panel with Reddit search, search still works.
// Every site: no console errors from our code; sign-in pages are never hidden.
import { createRequire } from 'node:module';
import { readFileSync, mkdirSync, writeFileSync, existsSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { tmpdir } from 'node:os';
import { loadPuppeteer } from '../../tools/puppeteer.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const KG = join(HERE, '..', '..');
const REPO = join(KG, '..');
const EXT = join(KG, 'dist', 'chromium-dev');
const puppeteer = loadPuppeteer();
if (existsSync(join(REPO, 'orchestration', 'LOW_RAM'))) { console.log('LOW_RAM is set: skipping the Chrome run'); process.exit(2); }
const DEV_ID = JSON.parse(readFileSync(join(KG, 'shared', 'ext-ids.json'), 'utf8')).chromium.devId;
const ORIGIN = `chrome-extension://${DEV_ID}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const results = [];
const check = (name, ok, detail = '') => { results.push({ name, ok: !!ok }); console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ` (${detail})` : ''}`); };
const note = (s) => console.log(`NOTE ${s}`);

const profile = join(tmpdir(), `kg-e2e-bs-${process.pid}`);
mkdirSync(join(profile, 'Default'), { recursive: true });
writeFileSync(join(profile, 'Default', 'Preferences'), JSON.stringify({ webkit: { webprefs: { default_font_size: 20, default_fixed_font_size: 17 } }, intl: { accept_languages: 'en-GB,en' } }));
const browser = await puppeteer.launch({ headless: true, pipe: true, enableExtensions: [EXT], userDataDir: profile, args: ['--no-first-run', '--lang=en-GB'] });
const ours = [];
let exitCode = 1;
try {
  const sw = await browser.waitForTarget((t) => t.type() === 'service_worker' && t.url().startsWith(`${ORIGIN}/`), { timeout: 15000 });
  (await sw.worker()).on('console', (m) => { if (m.type() === 'error') ours.push(`worker: ${m.text().slice(0, 160)}`); });
  await sleep(800);
  const hp = await browser.newPage();
  await hp.goto(`${ORIGIN}/pages/popup.html`);
  const r = await hp.evaluate(() => chrome.runtime.sendMessage({ type: 'kg:core:test', op: 'set-settings', settings: { ageBand: '13-15', bedtime: null, limits: { browseMin: null, videoMin: null } } }));
  await hp.close();
  check('band 13-15 set (social feed-free)', r && r.cfg && r.cfg.band && r.cfg.band.social === 'feedfree');
  await sleep(800);

  const state = (p) => p.evaluate(() => {
    const h = document.getElementById('kg-ff-panel');
    return {
      url: location.href, feed: document.documentElement.classList.contains('kg-ff-feed'),
      panel: !!h && h.getBoundingClientRect().height > 0,
      search: !!(h && h.shadowRoot && h.shadowRoot.querySelector('form[role="search"] input')),
      label: h && h.shadowRoot ? (h.shadowRoot.querySelector('label') || {}).textContent || '' : '',
      password: !!document.querySelector('input[type="password"]'),
      mainVisible: [...document.querySelectorAll('main, [role="main"], shreddit-feed')].filter((e) => e.getBoundingClientRect().height > 0).length,
      text: (document.body && document.body.innerText || '').slice(0, 120).replace(/\s+/g, ' '),
    };
  }).catch((e) => ({ error: String(e) }));
  async function visit(url, wait = 6000) {
    const p = await browser.newPage();
    p.on('console', (m) => { if (m.type() === 'error' && String((m.location() || {}).url || '').startsWith(ORIGIN)) ours.push(`${url}: ${m.text().slice(0, 160)}`); });
    p.on('pageerror', (e) => { const s = String((e && e.stack) || e); if (s.includes(ORIGIN)) ours.push(`${url}: ${s.slice(0, 160)}`); });
    await p.setViewport({ width: 1280, height: 900 });
    try { await p.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch (e) { note(`${url}: ${e.message.slice(0, 80)}`); }
    await sleep(wait);
    return p;
  }

  // Reddit: public home feed (Reddit may refuse a headless browser; then it is reported as not tested)
  const redditRefused = (s) => /blocked by network security/i.test(s.text || '');
  {
    const p = await visit('https://www.reddit.com/');
    const s = await state(p);
    note(`reddit / -> ${JSON.stringify(s)}`);
    if (redditRefused(s)) { note('Reddit refused this headless browser ("blocked by network security"): Reddit NOT TESTED in this run'); await p.close(); }
    else {
    check('Reddit home: feed hidden, panel with Reddit search in its place', s.feed && s.panel && s.search && s.mainVisible === 0 && /Reddit/.test(s.label), `${s.label}; main visible ${s.mainVisible}`);
    await p.evaluate(() => { const i = document.getElementById('kg-ff-panel').shadowRoot.querySelector('input'); i.value = 'volcanoes'; i.form.requestSubmit(); }).catch(() => {});
    await p.waitForNavigation({ timeout: 20000 }).catch(() => {});
    await sleep(5000);
    const s2 = await state(p);
    check('Reddit: our search box opens Reddit search, and results are not hidden', /reddit\.com\/search\/\?q=volcanoes/.test(s2.url) && !s2.feed && !s2.panel && s2.mainVisible > 0, s2.url);
    await p.goto('https://www.reddit.com/r/popular/', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await sleep(5000);
    const s3 = await state(p);
    check('Reddit r/popular counts as a feed', s3.feed && s3.panel, s3.url);
    await p.goto('https://www.reddit.com/r/space/', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await sleep(5000);
    const s4 = await state(p);
    check('Reddit: a community page the child picked stays open', !s4.feed && !s4.panel && s4.mainVisible > 0, s4.url);
    await p.close();
    }
  }

  // Best effort: what the others show logged out
  for (const [url, name] of [['https://www.tiktok.com/', 'TikTok'], ['https://www.tiktok.com/explore', 'TikTok explore'], ['https://www.instagram.com/', 'Instagram'], ['https://www.instagram.com/explore/', 'Instagram explore'],
    ['https://www.facebook.com/', 'Facebook'], ['https://www.facebook.com/reel/', 'Facebook reels'], ['https://x.com/explore', 'X explore'], ['https://www.pinterest.com/', 'Pinterest'], ['https://bsky.app/', 'Bluesky']]) {
    const p = await visit(url, 7000);
    const s = await state(p);
    note(`${name}: ${JSON.stringify(s)}`);
    if (s.password) check(`${name}: a sign-in page is left alone (no panel, nothing hidden)`, !s.panel, s.url);
    else if (s.feed) check(`${name}: feed path shows our panel and hides the feed area`, s.panel && s.mainVisible === 0, `${s.url}; main visible ${s.mainVisible}`);
    else note(`${name}: not a feed path after redirects (${s.url}); nothing to test`);
    await p.close();
  }

  check('no console errors from our code', ours.length === 0, ours.slice(0, 4).join(' | '));
  const failed = results.filter((x) => !x.ok);
  console.log(`\n${results.length - failed.length}/${results.length} passed`);
  exitCode = failed.length ? 1 : 0;
} catch (e) {
  console.error('E2E crashed:', e);
} finally {
  await browser.close().catch(() => {});
  rmSync(profile, { recursive: true, force: true });
  process.exit(exitCode);
}
