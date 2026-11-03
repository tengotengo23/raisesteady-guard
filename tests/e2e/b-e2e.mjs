#!/usr/bin/env node
// End-to-end: Chrome headless (new) with the unpacked dev build, standalone mode.
//   node tools/build-ext.mjs --dev && node tests/e2e/b-e2e.mjs
// Live: youtube.com (home, search, Shorts, watch) and the curated feeds. Local: an ad test page on
// ads.kgtest.example and a script from a listed ad domain (ib.adnxs.com), both mapped to a local server.
// Profile: Chrome font 20 / fixed 17 and 110 % zoom (large text); both motion modes.
import { createRequire } from 'node:module';
import { readFileSync, mkdirSync, writeFileSync, existsSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { loadPuppeteer } from '../../tools/puppeteer.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const KG = join(HERE, '..', '..');
const REPO = join(KG, '..');
const EXT = join(KG, 'dist', 'chromium-dev');
const FIX = join(HERE, 'fixtures-b');
const puppeteer = loadPuppeteer();
if (existsSync(join(REPO, 'orchestration', 'LOW_RAM'))) { console.log('LOW_RAM is set: skipping the Chrome run'); process.exit(2); }
if (!existsSync(join(EXT, 'manifest.json'))) { console.error('Build first: node tools/build-ext.mjs --dev'); process.exit(1); }

const DEV_ID = JSON.parse(readFileSync(join(KG, 'shared', 'ext-ids.json'), 'utf8')).chromium.devId;
const ORIGIN = `chrome-extension://${DEV_ID}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const results = [];
const check = (name, ok, detail = '') => { results.push({ name, ok: !!ok }); console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ` (${detail})` : ''}`); };
const ONLY = process.argv.slice(2).filter((a) => !a.startsWith('-'));
const want = (k) => !ONLY.length || ONLY.includes(k);

// ---- local server: the ad test page and the "ad" script ----
const server = createServer((req, res) => {
  const host = String(req.headers.host || '').split(':')[0];
  if (host === 'ib.adnxs.com') { res.writeHead(200, { 'content-type': 'text/javascript' }); return res.end('window.__kgAdScript = true; document.body.insertAdjacentHTML("beforeend", "<div id=\\"from-ad-script\\">ad from script</div>");'); }
  if (req.url === '/favicon.ico') { res.writeHead(404); return res.end(); }
  res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
  res.end(readFileSync(join(FIX, 'ads.html')));
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const PORT = server.address().port;

const profile = join(tmpdir(), `kg-e2e-b-${process.pid}`);
mkdirSync(join(profile, 'Default'), { recursive: true });
writeFileSync(join(profile, 'Default', 'Preferences'), JSON.stringify({
  webkit: { webprefs: { default_font_size: 20, default_fixed_font_size: 17 } },
  partition: { default_zoom_level: { x: Math.log(1.1) / Math.log(1.2) } },
  intl: { accept_languages: 'en-GB,en' },
}));

const browser = await puppeteer.launch({
  headless: true, pipe: true, enableExtensions: [EXT], userDataDir: profile,
  args: [
    `--host-resolver-rules=MAP ads.kgtest.example 127.0.0.1:${PORT}, MAP ib.adnxs.com 127.0.0.1:${PORT}`,
    '--disable-features=HttpsUpgrades,HttpsFirstBalancedModeAutoEnable',
    '--no-first-run', '--no-default-browser-check', '--lang=en-GB', '--autoplay-policy=no-user-gesture-required',
  ],
});

// Console errors from our code only (YouTube logs plenty of its own).
const ours = [];
const isOurs = (text, url) => String(url || '').startsWith(ORIGIN) || /\[kg\]/.test(text);
function watchConsole(p, label) {
  p.on('console', (m) => { if (m.type() === 'error' && isOurs(m.text(), (m.location() || {}).url)) ours.push(`${label}: ${m.text().slice(0, 200)}`); });
  p.on('pageerror', (e) => { const s = String((e && e.stack) || e); if (s.includes(ORIGIN) || s.includes('kg-')) ours.push(`${label}: ${s.slice(0, 200)}`); });
}

let exitCode = 1;
try {
  const sw = await browser.waitForTarget((t) => t.type() === 'service_worker' && t.url().startsWith(`${ORIGIN}/`), { timeout: 15000 });
  check('extension loads (dev id)', !!sw);
  const worker = await sw.worker();
  worker.on('console', (m) => { if (m.type() === 'error') ours.push(`worker: ${m.text().slice(0, 200)}`); });
  await sleep(800);

  const hook = async (msg) => {
    const p = await browser.newPage();
    await p.goto(`${ORIGIN}/pages/popup.html`);
    const r = await p.evaluate((m) => chrome.runtime.sendMessage(m), msg);
    await p.close();
    return r;
  };
  const NO_LIMITS = { bedtime: null, limits: { browseMin: null, videoMin: null } };
  const setBand = async (ageBand, extra = {}) => {
    const r = await hook({ type: 'kg:core:test', op: 'set-settings', settings: { ageBand, ...NO_LIMITS, ...extra } });
    await sleep(700); // storage.local kg.b.focus / kg.b.adblock reach the tabs
    return r;
  };
  const r0 = await setBand('9-12');
  check('test hook sets the band (9-12, no limits for the test)', r0 && r0.cfg && r0.cfg.ageBand === '9-12', JSON.stringify(r0 && r0.cfg && r0.cfg.ageBand));

  async function consent(p) {
    if (!/consent\.(youtube|google)\./.test(p.url())) return false;
    const clicked = await p.evaluate(() => {
      const b = [...document.querySelectorAll('button, input[type=submit]')].find((x) => /reject all|accept all/i.test(x.innerText || x.value || x.getAttribute('aria-label') || ''));
      if (b) { b.click(); return true; }
      return false;
    });
    await p.waitForNavigation({ timeout: 15000 }).catch(() => {});
    return clicked;
  }
  async function open(url, wait = 3000, label = 'page') {
    const p = await browser.newPage();
    watchConsole(p, label);
    await p.setViewport({ width: 1280, height: 900 });
    try { await p.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch { /* redirects can abort */ }
    if (await consent(p)) { try { await p.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch { /* fine */ } }
    await sleep(wait);
    return p;
  }
  const waitFor = async (p, fn, arg, ms = 20000) => { try { await p.waitForFunction(fn, { timeout: ms, polling: 250 }, arg); return true; } catch { return false; } };

  // ---- 1. new tab grid ----
  if (want('newtab')) {
    const p = await open(`${ORIGIN}/pages/newtab.html`, 0, 'newtab');
    const ok = await waitFor(p, () => document.querySelectorAll('#videoGrid .kgv-card').length >= 4, null, 30000);
    const g = await p.evaluate(() => {
      const cards = [...document.querySelectorAll('#videoGrid .kgv-card')];
      return {
        n: cards.length, shown: !document.getElementById('vids').hidden,
        thumbs: cards.every((a) => /^https:\/\/i\.ytimg\.com\/vi\/[\w-]{11}\/mqdefault\.jpg$/.test(a.querySelector('img').src)),
        links: cards.every((a) => /^https:\/\/www\.youtube\.com\/watch\?v=[\w-]{11}$/.test(a.href)),
        titles: cards.every((a) => a.querySelector('.kgv-title').textContent.trim().length > 0 && a.querySelector('.kgv-ch').textContent.trim().length > 0),
        channels: new Set(cards.map((a) => a.querySelector('.kgv-ch').textContent)).size,
        label: cards[0] && cards[0].getAttribute('aria-label'),
      };
    });
    check('new tab: curated grid renders', ok && g.shown, `${g.n} cards`);
    check('new tab: i.ytimg.com thumbnails, watch links, title and channel', g.thumbs && g.links && g.titles, g.label);
    check('new tab: channels are mixed (no single channel fills it)', g.channels >= Math.min(4, g.n), `${g.channels} channels in ${g.n}`);
    await p.keyboard.press('Tab');
    let reached = false;
    for (let i = 0; i < 40 && !reached; i++) { reached = await p.evaluate(() => !!(document.activeElement && document.activeElement.classList.contains('kgv-card'))); if (!reached) await p.keyboard.press('Tab'); }
    check('new tab: video cards are reachable by keyboard', reached);
    await p.close();
    // Focus mode off: the new tab page shows no videos and asks YouTube for nothing.
    await setBand('9-12', { focusMode: false });
    const off = await browser.newPage();
    const yt = [];
    off.on('request', (q) => { if (/youtube\.com|ytimg\.com/.test(q.url())) yt.push(q.url()); });
    await off.goto(`${ORIGIN}/pages/newtab.html`);
    await sleep(3000);
    const hidden = await off.evaluate(() => document.getElementById('vids').hidden && !document.querySelector('#videoGrid .kgv-card'));
    check('new tab, focus mode off: no videos and no request to YouTube', hidden && yt.length === 0, `${yt.length} YouTube requests`);
    await off.close();
    await setBand('9-12');
  }

  // ---- 2. YouTube home ----
  let home = null;
  if (want('youtube')) {
    const p = await browser.newPage();
    watchConsole(p, 'yt-home');
    await p.setViewport({ width: 1280, height: 900 });
    // Record, from the first script on, whether any feed item is ever visible (no flash of the feed).
    await p.evaluateOnNewDocument(() => {
      window.__kgFeedSeen = 0; window.__kgCssAtStart = null;
      const tickCheck = () => {
        if (window.__kgCssAtStart === null && document.documentElement) window.__kgCssAtStart = !!document.getElementById('kg-yt-css') && document.documentElement.classList.contains('kg-f');
        for (const e of document.querySelectorAll('ytd-rich-item-renderer, ytd-rich-grid-renderer ytd-rich-grid-row, ytd-browse[page-subtype="home"] ytd-two-column-browse-results-renderer')) {
          const r = e.getBoundingClientRect();
          if (r.height > 0 && r.width > 0) { window.__kgFeedSeen++; break; }
        }
        if (performance.now() < 15000) requestAnimationFrame(tickCheck);
      };
      document.addEventListener('DOMContentLoaded', () => { if (window.__kgCssAtStart === null) window.__kgCssAtStart = !!document.getElementById('kg-yt-css') && document.documentElement.classList.contains('kg-f'); });
      requestAnimationFrame(tickCheck);
    });
    try { await p.goto('https://www.youtube.com/', { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch { /* fine */ }
    if (await consent(p)) { try { await p.goto('https://www.youtube.com/', { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch { /* fine */ } }
    const ok = await waitFor(p, () => { const h = document.getElementById('kg-yt-home'); return h && h.shadowRoot && h.shadowRoot.querySelectorAll('a.card').length >= 4; }, null, 30000);
    await sleep(1500);
    const s = await p.evaluate(() => {
      const h = document.getElementById('kg-yt-home');
      const sr = h && h.shadowRoot;
      const browse = h && h.parentElement;
      const others = browse ? [...browse.children].filter((c) => c !== h && c.getBoundingClientRect().height > 0).map((c) => c.tagName) : ['no browse'];
      return {
        inBrowse: !!browse && browse.tagName === 'YTD-BROWSE' && browse.getAttribute('page-subtype') === 'home',
        input: !!(sr && sr.querySelector('form[role="search"] input[name="search_query"]')),
        cards: sr ? sr.querySelectorAll('a.card').length : 0,
        others, feedSeen: window.__kgFeedSeen, cssAtStart: window.__kgCssAtStart,
        richVisible: [...document.querySelectorAll('ytd-rich-item-renderer')].filter((e) => e.getBoundingClientRect().height > 0).length,
        shortsNav: [...document.querySelectorAll('ytd-guide-entry-renderer, ytd-mini-guide-entry-renderer')].filter((e) => /Shorts/.test(e.textContent) && e.getBoundingClientRect().height > 0).length,
        links: sr ? [...sr.querySelectorAll('a.card')].every((a) => /^\/watch\?v=[\w-]{11}$/.test(a.getAttribute('href'))) : false,
      };
    });
    check('YouTube home: large search box and curated videos', ok && s.input && s.cards >= 4, `${s.cards} cards`);
    check('YouTube home: no feed (nothing else visible in the home page)', s.inBrowse && s.others.length === 0 && s.richVisible === 0, `others=${s.others.join(',')}`);
    check('YouTube home: hiding CSS in place at document start; no feed item ever painted', s.cssAtStart === true && s.feedSeen === 0, `css=${s.cssAtStart} seen=${s.feedSeen}`);
    check('YouTube home: Shorts entry gone from the guide', s.shortsNav === 0);
    check('YouTube home: cards open watch pages', s.links);
    home = p;
  }

  // ---- 3. search from our box ----
  if (want('youtube') && home) {
    const p = home;
    await p.evaluate(() => { const i = document.getElementById('kg-yt-home').shadowRoot.querySelector('input'); i.focus(); });
    await p.keyboard.type('volcano');
    await Promise.all([p.waitForNavigation({ timeout: 20000 }).catch(() => {}), p.keyboard.press('Enter')]);
    const ok = await waitFor(p, () => [...document.querySelectorAll('ytd-video-renderer, yt-lockup-view-model')].filter((e) => e.getBoundingClientRect().height > 0).length >= 3, null, 20000);
    const r = await p.evaluate(() => ({
      url: location.href,
      vids: [...document.querySelectorAll('ytd-video-renderer, yt-lockup-view-model')].filter((e) => e.getBoundingClientRect().height > 0).length,
      shorts: [...document.querySelectorAll('a[href^="/shorts/"]')].filter((e) => e.getBoundingClientRect().height > 0).length,
      shelves: [...document.querySelectorAll('ytd-reel-shelf-renderer, grid-shelf-view-model')].filter((e) => e.getBoundingClientRect().height > 0).length,
      note: !!document.querySelector('#kg-yt-note') && document.querySelector('#kg-yt-note').getBoundingClientRect().height > 0,
    }));
    check('search from our box works (results page with videos)', ok && /\/results\?search_query=volcano/.test(r.url), `${r.vids} videos`);
    check('search results: no Shorts links or shelves visible', r.shorts === 0 && r.shelves === 0, `links=${r.shorts} shelves=${r.shelves}`);
    check('search results (9-12): no curated-only note', !r.note);
    await p.close();
  }

  // ---- 4. Shorts address ----
  if (want('youtube')) {
    const p = await open('https://www.youtube.com/shorts/r65hiAj3k94', 2500, 'yt-shorts');
    check('a /shorts/ address opens as a normal watch page', /\/watch\?v=r65hiAj3k94/.test(p.url()), p.url());
    await p.close();
  }

  // ---- 4b. channel page, SPA navigation, mobile site ----
  if (want('youtube')) {
    const p = await open('https://www.youtube.com/@SesameStreet', 4000, 'yt-channel');
    const ch = await p.evaluate(() => ({
      tabs: [...document.querySelectorAll('yt-tab-shape')].filter((t) => t.getBoundingClientRect().height > 0).map((t) => t.textContent.trim()),
      shorts: [...document.querySelectorAll('a[href^="/shorts/"]')].filter((e) => e.getBoundingClientRect().height > 0).length,
    }));
    check('channel page: no Shorts tab, no Shorts links', ch.tabs.length > 1 && !ch.tabs.includes('Shorts') && ch.shorts === 0, ch.tabs.join(','));
    await p.goto('https://www.youtube.com/@SesameStreet/shorts', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await sleep(2500);
    check('channel /shorts tab address opens the Videos tab', /\/@SesameStreet\/videos$/.test(p.url()), p.url());
    // Endless grids and lists beyond home (from the review)
    const panelState = () => p.evaluate(() => {
      const h = document.getElementById('kg-yt-home');
      const browse = document.querySelector('ytd-page-manager > ytd-browse:not([hidden])');
      const shown = (e) => !!e && e.getBoundingClientRect().height > 0;
      return {
        panel: shown(h), input: !!(h && h.shadowRoot && h.shadowRoot.querySelector('form[role="search"] input')),
        cards: h && h.shadowRoot ? h.shadowRoot.querySelectorAll('a.card').length : 0, mode: h && h.dataset.mode,
        others: browse ? [...browse.children].filter((c) => c !== h && shown(c)).map((c) => c.id || c.tagName) : [],
        firstShown: browse ? ([...browse.children].find((c) => shown(c)) || {}).id || '' : '',
        afterHeader: !!(h && h.previousElementSibling && h.previousElementSibling.id === 'header'),
      };
    });
    await p.goto('https://www.youtube.com/@SesameStreet', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await waitFor(p, () => { const h = document.getElementById('kg-yt-home'); return h && h.getBoundingClientRect().height > 0; }, null, 15000);
    const chome = await panelState();
    check('channel home: header and tabs stay, home grid hidden, our search box under the tabs', chome.panel && chome.input && chome.cards === 0 && chome.afterHeader && chome.others.join(',') === 'header', JSON.stringify(chome));
    await p.goto('https://www.youtube.com/@SesameStreet/videos', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await sleep(3500);
    const vids = await panelState();
    check('channel Videos tab: left as it is (no panel)', !vids.panel && vids.others.length >= 1, JSON.stringify(vids));
    await p.goto('https://www.youtube.com/feed/subscriptions', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await waitFor(p, () => { const h = document.getElementById('kg-yt-home'); return h && h.shadowRoot && h.shadowRoot.querySelectorAll('a.card').length >= 4; }, null, 15000);
    const subs = await panelState();
    check('subscriptions feed: replaced by our search box and curated videos', subs.panel && subs.input && subs.cards >= 4 && subs.others.length === 0, JSON.stringify(subs));
    for (const [url, label] of [['https://www.youtube.com/feed/history', 'history'], ['https://www.youtube.com/playlist?list=PL8TioFHubWFtUQTzELOjG9B-8Ldn1-5uJ', 'playlist']]) {
      await p.goto(url, { waitUntil: 'domcontentloaded' }).catch(() => {});
      await waitFor(p, () => { const h = document.getElementById('kg-yt-home'); return h && h.getBoundingClientRect().height > 0; }, null, 15000);
      await sleep(1500);
      const st = await panelState();
      check(`${label}: kept, with our search box above it`, st.panel && st.input && st.cards === 0 && st.firstShown === 'kg-yt-home' && st.others.length >= 1, JSON.stringify(st));
    }
    await p.goto('https://www.youtube.com/watch?v=aqz-KE-bpKQ', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await sleep(4000);
    await p.click('ytd-topbar-logo-renderer a, a#logo').catch(() => {});
    const spa = await waitFor(p, () => { const h = document.getElementById('kg-yt-home'); return location.pathname === '/' && h && h.getBoundingClientRect().height > 100 && h.shadowRoot.querySelectorAll('a.card').length >= 4; }, null, 15000);
    const vis = await p.evaluate(() => [...document.querySelectorAll('ytd-page-manager > ytd-browse:not([hidden]) > *')].filter((e) => e.getBoundingClientRect().height > 0).map((e) => e.id || e.tagName));
    check('single-page navigation (watch page, then the logo): home shows only our panel', spa && vis.length === 1 && vis[0] === 'kg-yt-home', vis.join(','));
    await p.close();
    const m = await browser.newPage();
    watchConsole(m, 'm-youtube');
    await m.setUserAgent('Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36');
    await m.setViewport({ width: 390, height: 844, isMobile: true, hasTouch: true });
    try { await m.goto('https://m.youtube.com/', { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch { /* fine */ }
    await consent(m);
    const mok = await waitFor(m, () => { const h = document.getElementById('kg-yt-home'); return h && h.shadowRoot && h.shadowRoot.querySelectorAll('a.card').length >= 4; }, null, 30000);
    const mh = await m.evaluate(() => ({
      feed: [...document.querySelectorAll('ytm-rich-item-renderer, ytm-rich-grid-renderer')].filter((e) => e.getBoundingClientRect().height > 0).length,
      pivotShorts: [...document.querySelectorAll('ytm-pivot-bar-item-renderer')].filter((e) => /Shorts/.test(e.textContent) && e.getBoundingClientRect().height > 0).length,
    }));
    check('m.youtube.com home: our panel, no feed, no Shorts button', mok && mh.feed === 0 && mh.pivotShorts === 0, JSON.stringify(mh));
    try { await m.goto('https://m.youtube.com/watch?v=aqz-KE-bpKQ', { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch { /* fine */ }
    await sleep(6000);
    const mw = await m.evaluate(() => [...document.querySelectorAll('ytm-item-section-renderer[section-identifier="related-items"]')].filter((e) => e.getBoundingClientRect().height > 0).length);
    check('m.youtube.com watch page: no related videos', mw === 0, `${mw}`);
    await m.close();
  }

  // ---- 5. watch page ----
  if (want('youtube')) {
    const p = await open('https://www.youtube.com/watch?v=aqz-KE-bpKQ', 1000, 'yt-watch');
    const playing = await waitFor(p, () => { const v = document.querySelector('#movie_player video'); return v && v.currentTime > 2 && !v.error; }, null, 25000);
    const w = await p.evaluate(() => {
      const shown = (s) => [...document.querySelectorAll(s)].filter((e) => e.getBoundingClientRect().height > 0).length;
      const ipr = window.ytInitialPlayerResponse || {}, ind = window.ytInitialData || {};
      const po = (ind.playerOverlays || {}).playerOverlayRenderer || {};
      const sheet = [...document.styleSheets].find((s) => s.ownerNode && s.ownerNode.id === 'kg-yt-css');
      const endRule = sheet && [...sheet.cssRules].some((r) => /\.html5-endscreen/.test(r.selectorText || '') && /none/.test(r.style.display));
      return {
        related: shown('#related, ytd-watch-next-secondary-results-renderer'),
        endscreenData: !!ipr.endscreen || !!po.endScreen, autoplayData: !!(((ind.contents || {}).twoColumnWatchNextResults || {}).autoplay) || !!po.autoplay,
        endShown: shown('.html5-endscreen, .ytp-endscreen-content, .ytp-ce-element, .ytp-autonav-endscreen'), endRule,
        autonavOn: [...document.querySelectorAll('.ytp-autonav-toggle-button')].some((b) => b.getAttribute('aria-checked') === 'true'),
        ads: !!ipr.adPlacements || !!ipr.playerAds || !!ipr.adSlots,
        comments: shown('ytd-comments#comments'), note: shown('#kg-yt-com'),
        noteText: (document.getElementById('kg-yt-com') || {}).textContent || '',
      };
    });
    check('watch page: video plays (our hooks did not break playback)', playing);
    check('watch page: no related-videos column', w.related === 0, `${w.related}`);
    check('watch page: no end-screen suggestions (data removed, CSS hides the wall)', !w.endscreenData && w.endShown === 0 && w.endRule, JSON.stringify({ d: w.endscreenData, s: w.endShown, r: w.endRule }));
    check('watch page: autoplay off', !w.autonavOn && !w.autoplayData, JSON.stringify({ toggle: w.autonavOn, data: w.autoplayData }));
    check('watch page: ad fields removed from the player data', !w.ads);
    await waitFor(p, () => document.getElementById('kg-yt-com'), null, 8000);
    const c = await p.evaluate(() => ({ comments: [...document.querySelectorAll('ytd-comments#comments')].filter((e) => e.getBoundingClientRect().height > 0).length, note: (document.getElementById('kg-yt-com') || {}).textContent || '' }));
    check('watch page (9-12): comments hidden, one plain line says so', c.comments === 0 && /switched off/.test(c.note), c.note);
    await setBand('13-15');
    await p.reload({ waitUntil: 'domcontentloaded' }).catch(() => {});
    await sleep(4000);
    const older = await p.evaluate(() => ({ nocom: document.documentElement.classList.contains('kg-nocom'), related: [...document.querySelectorAll('#related')].filter((e) => e.getBoundingClientRect().height > 0).length }));
    check('watch page (13-15): comments allowed, sidebar still hidden', !older.nocom && older.related === 0, JSON.stringify(older));
    await p.close();
  }

  // ---- 6. curated mode (3-5) ----
  if (want('youtube')) {
    await setBand('3-5');
    const p = await open('https://www.youtube.com/', 1000, 'yt-curated');
    await waitFor(p, () => { const h = document.getElementById('kg-yt-home'); return h && h.shadowRoot && h.shadowRoot.querySelector('.note') && h.shadowRoot.querySelectorAll('a.card').length >= 2; }, null, 30000);
    const hn = await p.evaluate(() => { const sr = document.getElementById('kg-yt-home').shadowRoot; return { note: (sr.querySelector('.note') || {}).textContent || '', cards: sr.querySelectorAll('a.card').length }; });
    check('3-5 home: curated videos and the one-line note under the search box', hn.cards >= 2 && /only from the channels/.test(hn.note), `${hn.cards} cards; ${hn.note}`);
    await p.goto('https://www.youtube.com/results?search_query=elmo', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await waitFor(p, () => document.querySelectorAll('[data-kg-ok]').length >= 1, null, 20000);
    await sleep(1500);
    const cur = await p.evaluate(() => {
      const vis = [...document.querySelectorAll('ytd-search ytd-video-renderer, ytd-search yt-lockup-view-model')].filter((e) => e.getBoundingClientRect().height > 0);
      const anyVisible = [...document.querySelectorAll('ytd-search ytd-item-section-renderer #contents > *')].filter((e) => e.getBoundingClientRect().height > 0);
      const note = document.getElementById('kg-yt-note');
      return { vis: vis.length, allOk: vis.every((e) => e.hasAttribute('data-kg-ok')), other: anyVisible.filter((e) => !e.hasAttribute('data-kg-ok')).length,
        chans: [...new Set(vis.map((e) => (e.querySelector('a[href^="/@"], a[href^="/channel/"]') || {}).getAttribute?.('href')))].slice(0, 5),
        note: note ? note.textContent : '', noteShown: !!note && note.getBoundingClientRect().height > 0 };
    });
    check('3-5 search: only videos from curated channels are visible', cur.vis >= 1 && cur.allOk && cur.other === 0, `${cur.vis} visible from ${cur.chans.join(' ')}`);
    check('3-5 search: one plain line says so, in the page flow', cur.noteShown && /only from the channels/.test(cur.note), cur.note);
    // watch pages: a curated channel plays, another channel goes to the block page
    const curVid = await hook({ type: 'kg:focus:videos', ageBand: '3-5', lang: 'en', limit: 8 }).then((r) => r && r.videos && (r.videos.find((v) => !/\blive\b|premiere/i.test(v.title)) || r.videos[0])).catch(() => null);
    if (curVid) {
      await p.goto(`https://www.youtube.com/watch?v=${curVid.id}`, { waitUntil: 'domcontentloaded' }).catch(() => {});
      const plays = await waitFor(p, () => { const v = document.querySelector('#movie_player video'); return !document.documentElement.classList.contains('kg-hold') && v && v.currentTime > 1; }, null, 25000);
      check('3-5 watch page of a curated channel: shown and plays', plays && /\/watch\?v=/.test(p.url()), `${curVid.channel}: ${p.url()}`);
    } else check('3-5 watch page of a curated channel: shown and plays', false, 'no curated video from the background');
    await p.goto('https://www.youtube.com/watch?v=aqz-KE-bpKQ', { waitUntil: 'domcontentloaded' }).catch(() => {});
    const blocked = await waitFor(p, (o) => location.href.startsWith(o), `${ORIGIN}/pages/blocked.html?reason=curated`, 15000);
    check('3-5 watch page of another channel (Blender): block page, reason curated', blocked, p.url());
    await p.goto('https://www.youtube.com/shorts/r65hiAj3k94', { waitUntil: 'domcontentloaded' }).catch(() => {});
    await sleep(2500);
    check('3-5: a /shorts/ address opens the block page', p.url().startsWith(`${ORIGIN}/pages/blocked.html?reason=shorts`), p.url());
    await p.close();
    await setBand('9-12');
  }

  // ---- 7. ad blocker on and off on a local test page ----
  if (want('ads')) {
    const look = (p) => p.evaluate(() => {
      const vis = (s) => { const e = document.querySelector(s); return !!e && getComputedStyle(e).display !== 'none' && e.getBoundingClientRect().height > 0; };
      return { ad1: vis('#AD_300'), ad2: vis('.ad-slot'), content: vis('#content'), script: !!window.__kgAdScript, fromScript: vis('#from-ad-script'), h: document.documentElement.getAttribute('data-kg-h') };
    });
    let p = await open('http://ads.kgtest.example/', 1200, 'ads-on');
    const on = await look(p);
    check('blocker on: generic EasyList elements hidden, content shown', !on.ad1 && !on.ad2 && on.content, JSON.stringify(on));
    check('blocker on: script from a listed ad domain blocked', !on.script && !on.fromScript);
    check('cosmetic.js writes the host list', on.h === 'ads.kgtest.example kgtest.example example', on.h);
    await setBand('9-12', { adblock: false });
    await sleep(500);
    const live = await look(p);
    check('blocker switched off: hidden elements come back on the open page', live.ad1 && live.ad2, JSON.stringify(live));
    await p.close();
    p = await open('http://ads.kgtest.example/', 1200, 'ads-off');
    const off = await look(p);
    check('blocker off: ads show and the ad script loads', off.ad1 && off.ad2 && off.script && off.fromScript, JSON.stringify(off));
    await p.close();
    await setBand('9-12');
    p = await open('http://ads.kgtest.example/', 1200, 'ads-on-again');
    const again = await look(p);
    check('blocker on again: hidden and blocked', !again.ad1 && !again.script, JSON.stringify(again));
    // every block of cosmetic.css parsed (one bad selector would drop its block)
    const parsed = await p.evaluate(async (url) => {
      const css = await (await fetch(url)).text().catch(() => null);
      return css;
    }, `${ORIGIN}/rules/cosmetic.css`).catch(() => null);
    const cssText = readFileSync(join(EXT, 'rules', 'cosmetic.css'), 'utf8');
    const counts = await p.evaluate((txt) => {
      const sh = new CSSStyleSheet(); sh.replaceSync(txt);
      let n = 0; const walk = (rules) => { for (const r of rules) { if (r.cssRules && r.cssRules.length) walk(r.cssRules); else n++; } };
      walk(sh.cssRules);
      return { top: sh.cssRules.length, leaves: n };
    }, cssText);
    const expected = (cssText.match(/\{ display: none !important; \}/g) || []).length;
    check('cosmetic.css: every block parses in Chrome', counts.leaves === expected, `${counts.leaves}/${expected} blocks, ${counts.top} top rules${parsed ? '' : ', not web-accessible (as intended)'}`);
    await p.close();
  }

  // ---- 8. layout: new tab grid and YouTube panel, 360 and 1280, both motion modes ----
  if (want('layout')) {
    const targets = [
      { name: 'newtab', url: `${ORIGIN}/pages/newtab.html`, root: () => document, sel: '.kgv-card', ready: () => document.querySelectorAll('.kgv-card').length >= 2 },
      { name: 'yt-home', url: 'https://www.youtube.com/', ready: () => { const h = document.getElementById('kg-yt-home'); return h && h.shadowRoot && h.shadowRoot.querySelectorAll('a.card').length >= 2; } },
    ];
    for (const tg of targets) {
      for (const width of [360, 1280]) {
        for (const motion of ['reduce', 'no-preference']) {
          const p = await browser.newPage();
          watchConsole(p, `${tg.name}-${width}-${motion}`);
          await p.setViewport({ width, height: 900 });
          await p.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: motion }]);
          try { await p.goto(tg.url, { waitUntil: 'domcontentloaded', timeout: 30000 }); } catch { /* fine */ }
          await consent(p);
          const ready = await waitFor(p, tg.ready, null, 30000);
          await sleep(600);
          const m = await p.evaluate((name) => {
            const yt = name === 'yt-home';
            const sr = yt ? document.getElementById('kg-yt-home').shadowRoot : document;
            const box = yt ? sr.querySelector('.p') : document.getElementById('videoGrid');
            const vw = document.documentElement.clientWidth;
            const r = box.getBoundingClientRect();
            const small = [...sr.querySelectorAll(yt ? 'a.card, input, button' : '.kgv-card')].filter((e) => { const b = e.getBoundingClientRect(); return b.height < 44 || b.width < 44; }).length;
            const over = [...sr.querySelectorAll(yt ? '.ti, .ch, h1, h2, .from, button' : '.kgv-title, .kgv-ch')].filter((e) => e.scrollWidth > e.clientWidth + 1).length;
            const fs = parseFloat(getComputedStyle(yt ? sr.querySelector('.ti') : document.querySelector('.kgv-title')).fontSize);
            const anim = yt ? getComputedStyle(box).animationName : getComputedStyle(document.querySelector('main')).animationName;
            const sideScroll = yt ? box.scrollWidth > box.clientWidth + 1 || r.right > vw + 1 : document.documentElement.scrollWidth > vw + 1;
            return { small, over, fs, anim, sideScroll, vw, dpr: devicePixelRatio };
          }, tg.name);
          const wantAnim = motion === 'reduce' ? 'kg-fade' : 'kg-rise';
          check(`${tg.name} ${width}px ${motion}: no side scroll, targets >= 44 px, no text overflow, text >= 16 px, ${wantAnim}`,
            ready && !m.sideScroll && m.small === 0 && m.over === 0 && m.fs >= 16 && m.anim === wantAnim,
            `css width ${m.vw}, dpr ${m.dpr}, font ${m.fs}px, anim ${m.anim}, small ${m.small}, overflow ${m.over}`);
          await p.close();
        }
      }
    }
  }

  check('no console errors from our code', ours.length === 0, ours.slice(0, 5).join(' | '));
  const failed = results.filter((r) => !r.ok);
  console.log(`\n${results.length - failed.length}/${results.length} passed`);
  exitCode = failed.length ? 1 : 0;
} catch (e) {
  console.error('E2E crashed:', e);
} finally {
  await browser.close().catch(() => {});
  server.close();
  rmSync(profile, { recursive: true, force: true });
  process.exit(exitCode);
}
