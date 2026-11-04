#!/usr/bin/env node
// End-to-end, P2-161 (0.3.1): Chrome headless with the unpacked dev build in managed mode, talking to the real
// Go agent API and parent page (TestServeForE2E on 127.0.0.1:47617, PIN 2468).
//   node tools/build-ext.mjs --dev && node tests/e2e/d-topics.mjs
// Covers:
//  Y1 a YouTube-like page (fixtures-d/yt-shell.html on www.youtube.com): only our search bar shows (YouTube's box,
//     voice button, suggestions and narrow search button hidden), its text is whole at Chrome font 20 / fixed 17
//     and 110 % zoom at 600, 1024, 1280 and 1920 px, nothing of ours under the fixed masthead, "/" focuses ours;
//  Y2 the grid with every channel feed failing (404) shows the built-in videos, "Showing saved videos", no offline text;
//  T1 topic searches (Google, DuckDuckGo with a spelling trick, our YouTube box), a topic page, a topic site and a
//     YouTube watch title go to the topic page with the helpline line; result titles are hidden; ordinary look-alike
//     pages and a helpline pass; the parent allows a page with the PIN; the parent page switch turns it all off.
// Both motion modes; no console errors. Every web host maps to a local server, so no real site is contacted.
// KG_SHOT_DIR: where the layout screenshots go (default: the system temp folder).
import { readFileSync, mkdirSync, writeFileSync, rmSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:http';
import { createServer as createHttpsServer } from 'node:https';
import { execFileSync, spawn } from 'node:child_process';
import { tmpdir } from 'node:os';
import { loadPuppeteer } from '../../tools/puppeteer.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const KG = join(HERE, '..', '..');
const REPO = join(KG, '..');
const EXT = join(KG, 'dist', 'chromium-dev');
const FIX = join(HERE, 'fixtures-d');
const SHOTS = process.env.KG_SHOT_DIR || tmpdir();
const puppeteer = loadPuppeteer();
if (existsSync(join(REPO, 'orchestration', 'LOW_RAM'))) { console.log('LOW_RAM is set: skipping the Chrome run'); process.exit(2); }
if (!existsSync(join(EXT, 'manifest.json'))) { console.error('Build first: node tools/build-ext.mjs --dev'); process.exit(1); }

const DEV_ID = JSON.parse(readFileSync(join(KG, 'shared', 'ext-ids.json'), 'utf8')).chromium.devId;
const ORIGIN = `chrome-extension://${DEV_ID}`;
const AGENT_PORT = 47617;
const AGENT = `http://127.0.0.1:${AGENT_PORT}`;
const TOKEN = 'a1'.repeat(32);
const PIN = '2468';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const results = [];
const check = (name, ok, detail = '') => { results.push({ name, ok: !!ok }); console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ` (${detail})` : ''}`); };

// ---- the agent test server (Go) ----
const agentProc = spawn('go', ['test', '-count=1', '-run', 'TestServeForE2E', './internal/api'], {
  cwd: join(KG, 'agent'), env: { ...process.env, KG_E2E_SECONDS: '300', KG_E2E_PORT: String(AGENT_PORT) }, stdio: 'ignore',
});
let up = false;
for (let i = 0; i < 240 && !up; i++) { try { up = (await fetch(`${AGENT}/parent`)).ok; } catch { await sleep(500); } }
if (!up) { console.error('the agent test server did not start'); agentProc.kill(); process.exit(1); }

// ---- local web server for every host ----
const SHELL = readFileSync(join(FIX, 'yt-shell.html'), 'utf8');
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
const ytItem = (id, title) => `<ytd-video-renderer data-id="${id}"><a id="video-title" href="/watch?v=${id}" title="${esc(title)}">${esc(title)}</a><ytd-channel-name><a href="/@learning">Learning channel</a></ytd-channel-name></ytd-video-renderer>`;
const WATCH = { 'aaaaaaaaaa1': 'Pride Month special: coming out stories', 'bbbbbbbbbb1': 'Pride and Prejudice, chapter one, read aloud' };
function ytPage(u) {
  const p = u.pathname;
  let page = '', title = 'YouTube', margin = 56;
  if (p === '/') page = '<ytd-browse page-subtype="home"><ytd-rich-grid-renderer><div class="feed-item">feed video</div><div class="feed-item">feed video</div></ytd-rich-grid-renderer></ytd-browse>';
  else if (p === '/feed/history') { margin = 0; page = '<ytd-browse page-subtype="history"><div class="feed-item">history</div></ytd-browse>'; } // overlap on purpose
  else if (p === '/results') {
    title = `${u.searchParams.get('search_query') || ''} - YouTube`;
    page = `<ytd-search><ytd-section-list-renderer><div id="contents"><ytd-item-section-renderer><div id="contents">${ytItem('cccccccccc1', 'How volcanoes work')}${ytItem('cccccccccc2', 'Transgender kids explained')}${ytItem('cccccccccc3', 'Volcano experiment for kids')}</div></ytd-item-section-renderer></div></ytd-section-list-renderer></ytd-search>`;
  } else if (p === '/watch') {
    const v = u.searchParams.get('v');
    title = `${WATCH[v] || 'A video'} - YouTube`;
    page = `<ytd-watch-flexy video-id="${esc(v)}"><ytd-watch-metadata><h1>${esc(WATCH[v] || 'A video')}</h1></ytd-watch-metadata></ytd-watch-flexy>`;
  }
  return SHELL.replace('__TITLE__', esc(title)).replace('__MARGIN__', String(margin)).replace('__PAGE__', page);
}
const PAGES = {
  'topic.kgtest.example': { title: 'Coming out as transgender: a guide for teens', h1: 'A guide' },
  'chem.kgtest.example': { title: "Gay-Lussac's law for kids", h1: 'Gases and pressure', desc: 'Pressure and temperature of a gas.' },
  'books.kgtest.example': { title: 'Pride and Prejudice: a summary for students', h1: 'Pride and Prejudice' },
  'findahelpline.com': { title: 'Find a helpline: support for LGBTQ+ young people', h1: 'Free, confidential support', desc: 'Helplines for gay, lesbian, bisexual and transgender young people.' },
  'glaad.org': { title: 'Home', h1: 'Welcome' },
};
const hits = { feeds: 0 };
const server = createServer((req, res) => {
  const host = String(req.headers.host || '').split(':')[0];
  const u = new URL(req.url, `https://${host}`);
  if (req.url === '/favicon.ico') { res.writeHead(204); return res.end(); }
  if (host === 'i.ytimg.com') { res.writeHead(200, { 'content-type': 'image/png' }); return res.end(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64')); }
  if (host === 'www.youtube.com' && u.pathname.startsWith('/feeds/')) { hits.feeds++; res.writeHead(404); return res.end('not found'); } // the outage
  if (host === 'www.youtube.com' && u.pathname.startsWith('/shorts/')) { res.writeHead(404); return res.end(); }
  res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
  if (host === 'www.youtube.com') return res.end(ytPage(u));
  if (u.pathname === '/words') { // adult words in the body: the adult-content check would flag it
    return res.end(`<!doctype html><title>Support and advice</title><h1 id="ok">Support</h1><p>${'porn xxx milf hentai nsfw live sex cams nude girls '.repeat(6)}</p>`);
  }
  if (host === 'spa.kgtest.example') { // a single-page site: address and title change without a reload
    return res.end(`<!doctype html><title>Start</title><h1 id="ok">Start</h1><script>setTimeout(() => { history.pushState({}, '', '/coming-out-as-transgender'); document.title = 'Coming out as transgender'; }, 1200);</script>`);
  }
  const pg = PAGES[host] || { title: `Results for ${u.searchParams.get('q') || ''}`, h1: host.replace(/[^a-z0-9.-]/gi, '') };
  res.end(`<!doctype html><html lang="en"><title>${esc(pg.title)}</title><meta name="description" content="${esc(pg.desc || '')}"><h1 id="ok">${esc(pg.h1)}</h1><p>Some text.</p></html>`);
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const certDir = join(tmpdir(), `kg-e2e-d-cert-${process.pid}`);
mkdirSync(certDir, { recursive: true });
execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-subj', '/CN=kgtest', '-keyout', join(certDir, 'k.pem'), '-out', join(certDir, 'c.pem')], { stdio: 'ignore' });
const tls = createHttpsServer({ key: readFileSync(join(certDir, 'k.pem')), cert: readFileSync(join(certDir, 'c.pem')) }, (req, res) => server.emit('request', req, res));
await new Promise((r) => tls.listen(0, '127.0.0.1', r));
const HTTPS_HOSTS = ['www.youtube.com', 'i.ytimg.com', 'www.google.com', 'duckduckgo.com', 'findahelpline.com', 'glaad.org'];
const HTTP_HOSTS = ['topic.kgtest.example', 'chem.kgtest.example', 'books.kgtest.example', 'words.kgtest.example', 'spa.kgtest.example'];

const profile = join(tmpdir(), `kg-e2e-d-${process.pid}`);
mkdirSync(join(profile, 'Default'), { recursive: true });
writeFileSync(join(profile, 'Default', 'Preferences'), JSON.stringify({
  webkit: { webprefs: { default_font_size: 20, default_fixed_font_size: 17 } },
  partition: { default_zoom_level: { x: Math.log(1.1) / Math.log(1.2) } },
  intl: { accept_languages: 'en-GB,en' },
}));
const browser = await puppeteer.launch({
  headless: true, pipe: true, enableExtensions: [EXT], userDataDir: profile,
  args: [
    `--host-resolver-rules=${[...HTTPS_HOSTS.map((h) => `MAP ${h} 127.0.0.1:${tls.address().port}`), ...HTTP_HOSTS.map((h) => `MAP ${h} 127.0.0.1:${server.address().port}`)].join(', ')}`,
    '--ignore-certificate-errors', '--disable-features=HttpsUpgrades,HttpsFirstBalancedModeAutoEnable',
    '--no-first-run', '--no-default-browser-check', '--lang=en-GB',
  ],
});

const errors = [];
function watch(p, label) {
  p.on('console', (m) => { if (m.type() === 'error') errors.push(`${label}: ${m.text().slice(0, 200)}`); });
  p.on('pageerror', (e) => errors.push(`${label}: ${String(e).slice(0, 200)}`));
}

let exitCode = 1;
try {
  const sw = await browser.waitForTarget((t) => t.type() === 'service_worker' && t.url().startsWith(`${ORIGIN}/`), { timeout: 15000 });
  const worker = await sw.worker();
  worker.on('console', (m) => { if (m.type() === 'error') errors.push(`worker: ${m.text().slice(0, 200)}`); });
  await sleep(800);
  const hook = async (msg) => {
    const p = await browser.newPage();
    await p.goto(`${ORIGIN}/pages/popup.html`);
    const r = await p.evaluate((m) => chrome.runtime.sendMessage(m), msg);
    await p.close();
    return r;
  };
  const sync = () => hook({ type: 'kg:core:test', op: 'sync' });
  const visit = async (url, label, waitMs = 1500, viewport = { width: 1280, height: 900 }) => {
    const p = await browser.newPage();
    watch(p, label);
    await p.setViewport(viewport);
    try { await p.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 }); } catch { /* the block redirect can abort the first load */ }
    await sleep(waitMs);
    return p;
  };
  const reasonOf = (u) => (String(u).startsWith(`${ORIGIN}/pages/blocked.html?`) ? new URLSearchParams(String(u).split('?')[1]).get('reason') : null);
  const waitReason = async (p, want, ms = 6000) => { for (let i = 0; i < ms / 200 && reasonOf(p.url()) !== want; i++) await sleep(200); return reasonOf(p.url()); };
  const fits = (p) => p.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1
    && [...document.querySelectorAll('button, input, label, p, h1, a')].every((e) => e.scrollWidth <= e.clientWidth + 2 || getComputedStyle(e).overflowX !== 'visible'));

  const parent = await browser.newPage();
  watch(parent, 'parent page');
  await parent.setViewport({ width: 1100, height: 900 });
  const submit = (sel) => Promise.all([parent.waitForNavigation({ waitUntil: 'load' }), parent.click(sel)]);
  const parentPage = async () => {
    await parent.goto(`${AGENT}/parent?lang=en`, { waitUntil: 'load' });
    if (await parent.$('#pin')) { await parent.type('#pin', PIN); await submit('form[action="/parent/login"] button[type=submit]'); }
    return parent.evaluate(() => document.body.innerText);
  };

  // 0. Managed mode, as the Windows policy sets it; the agent sends topics: true (an old config).
  await hook({ type: 'kg:core:test', op: 'set-managed', managed: { agentToken: TOKEN, agentPort: AGENT_PORT } });
  const r1 = await sync();
  check('managed mode: the extension reaches the agent; topic filter on by default', r1 && r1.agentOnline === true && r1.cfg && r1.cfg.topics === true, JSON.stringify({ online: r1 && r1.agentOnline, topics: r1 && r1.cfg && r1.cfg.topics }));

  // ---- Y1 + Y2: the YouTube-like home page, both motion modes, four widths ----
  const ytLayout = async (p) => p.evaluate(() => {
    const vis = (e) => { if (!e) return false; const cs = getComputedStyle(e); const r = e.getBoundingClientRect(); return cs.display !== 'none' && cs.visibility !== 'hidden' && r.width > 0 && r.height > 0; };
    const host = document.getElementById('kg-yt-home');
    const sr = host && host.shadowRoot;
    const input = sr && sr.querySelector('input');
    const btn = sr && sr.querySelector('button');
    const mast = document.getElementById('masthead-container').getBoundingClientRect();
    const ctx = document.createElement('canvas').getContext('2d');
    let phFits = false;
    if (input) {
      const cs = getComputedStyle(input);
      ctx.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`;
      const room = input.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
      phFits = ctx.measureText(input.placeholder).width <= room + 1;
    }
    const r = (e) => e && e.getBoundingClientRect();
    return {
      ours: vis(input), ytBox: vis(document.getElementById('yt-own-search')), voice: vis(document.getElementById('voice-search-button')),
      narrow: vis(document.getElementById('search-button-narrow')), sugg: vis(document.querySelector('.ytSearchboxComponentSuggestionsContainer')),
      chips: vis(document.getElementById('frosted-glass')),
      mSearch: vis(document.querySelector('ytm-mobile-topbar-renderer .topbar-button-search-button')),
      belowMast: !!input && r(host).top >= mast.bottom - 0.5 && r(sr.querySelector('h1') || input).top >= mast.bottom - 0.5,
      inView: !!input && r(btn).right <= document.documentElement.clientWidth + 0.5 && r(input).left >= 0,
      btnFits: !!btn && btn.scrollWidth <= btn.clientWidth + 1 && btn.scrollHeight <= btn.clientHeight + 1,
      phFits, inputFont: input ? parseFloat(getComputedStyle(input).fontSize) : 0,
      sideScroll: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
      cards: sr ? sr.querySelectorAll('a.card').length : 0,
      saved: sr && sr.querySelector('.saved') ? sr.querySelector('.saved').textContent : '',
      offline: sr ? /need the internet/.test(sr.textContent) : true,
      feed: [...document.querySelectorAll('.feed-item')].some(vis),
      dbg: `${location.href} cls=${document.documentElement.className} htmlVis=${getComputedStyle(document.documentElement).visibility} bodyVis=${document.body && getComputedStyle(document.body).visibility} host=${!!host}`,
    };
  });
  let shot = '';
  for (const motion of ['reduce', 'no-preference']) {
    for (const width of process.env.KG_QUICK ? [1280] : [1280, 1024, 1920, 600]) {
      const p = await browser.newPage();
      watch(p, `yt ${width} ${motion}`);
      await p.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: motion }]);
      await p.setViewport({ width, height: 900 });
      try { await p.goto('https://www.youtube.com/', { waitUntil: 'domcontentloaded', timeout: 15000 }); } catch { /* fine */ }
      await p.waitForFunction(() => { const h = document.getElementById('kg-yt-home'); return h && h.shadowRoot && (h.shadowRoot.querySelectorAll('a.card').length >= 4 || /internet|on their way/.test(h.shadowRoot.textContent)); }, { timeout: 25000, polling: 250 }).catch(() => {});
      await sleep(700);
      const s = await ytLayout(p);
      const tag = `@${width} ${motion}`;
      check(`Y1 ${tag}: only our search bar shows (YouTube's box, voice, suggestions, narrow button, chip bar hidden)`, s.ours && !s.ytBox && !s.voice && !s.sugg && !s.narrow && !s.chips && !s.mSearch, JSON.stringify({ ours: s.ours, yt: s.ytBox, voice: s.voice, sugg: s.sugg, narrow: s.narrow, chips: s.chips, m: s.mSearch, dbg: s.dbg }));
      check(`Y1 ${tag}: our box is whole and below the masthead (text fits, no side scroll)`, s.belowMast && s.inView && s.btnFits && s.phFits && !s.sideScroll && s.inputFont >= 20, JSON.stringify({ below: s.belowMast, view: s.inView, btn: s.btnFits, ph: s.phFits, font: s.inputFont, side: s.sideScroll }));
      if (width === 1280) {
        check(`Y2 ${tag}: feeds answer 404, the built-in videos show with "Showing saved videos", no offline text, no feed`, s.cards >= 4 && s.saved === 'Showing saved videos.' && !s.offline && !s.feed && hits.feeds > 0, `${s.cards} cards, feeds asked ${hits.feeds}`);
        if (motion === 'reduce') { shot = join(SHOTS, 'p2-161-yt-home-1280-font20-zoom110.png'); await p.screenshot({ path: shot }); }
      }
      if (width === 600 && motion === 'reduce') await p.screenshot({ path: join(SHOTS, 'p2-161-yt-home-600-font20-zoom110.png') });
      await p.close();
    }
  }
  console.log(`screenshot: ${shot}`);

  // "/" focuses our box, YouTube's shortcut never runs; typing "/" inside our box types it.
  {
    const p = await visit('https://www.youtube.com/', 'yt slash', 2500);
    await p.click('h1#nothing').catch(() => {});
    await p.keyboard.press('/');
    await sleep(200);
    const r = await p.evaluate(() => ({ ours: document.activeElement && document.activeElement.id === 'kg-yt-home' && document.getElementById('kg-yt-home').shadowRoot.activeElement?.id === 'kgq', yt: window.__ytSlash }));
    await p.keyboard.type('a/b');
    const val = await p.evaluate(() => document.getElementById('kg-yt-home').shadowRoot.getElementById('kgq').value);
    check('Y1: "/" focuses our search box, not YouTube\'s', r.ours && r.yt === 0 && val === 'a/b', JSON.stringify({ ...r, val }));
    // A topic search from our box goes to the topic page.
    await p.evaluate(() => { const i = document.getElementById('kg-yt-home').shadowRoot.getElementById('kgq'); i.value = ''; i.focus(); });
    await p.keyboard.type('coming out as gay');
    await p.keyboard.press('Enter');
    check('T1: a topic search from our YouTube box goes to the topic page', await waitReason(p, 'topic') === 'topic', p.url());
    await p.close();
  }

  // Nothing of ours under the masthead even when the page starts under it (a list page, page manager without margin).
  {
    const p = await visit('https://www.youtube.com/feed/history', 'yt history', 2500);
    const r = await p.evaluate(() => { const h = document.getElementById('kg-yt-home'); const i = h && h.shadowRoot.querySelector('input'); return { top: i ? i.getBoundingClientRect().top : -1, mast: document.getElementById('masthead-container').getBoundingClientRect().bottom, yt: getComputedStyle(document.getElementById('yt-own-search')).visibility }; });
    check('Y1: on a page that starts under the masthead, our box moves below it', r.top >= r.mast && r.yt === 'hidden', JSON.stringify(r));
    await p.close();
  }

  // ---- T1 with the filter on ----
  const topicPage = async (p, label) => {
    const reason = await waitReason(p, 'topic');
    await p.waitForSelector('#help:not([hidden])', { timeout: 4000 }).catch(() => {});
    const t = await p.evaluate(() => ({ title: document.getElementById('title').textContent, body: document.getElementById('body').textContent, link: document.getElementById('helpLink')?.href, help: !document.getElementById('help').hidden }));
    check(`T1: ${label} goes to the topic page with the helpline line`, reason === 'topic' && t.title === 'This topic is for adults.' && t.body === 'If you need help, you can always talk to someone:' && t.help && t.link === 'https://findahelpline.com/', JSON.stringify({ reason, ...t }));
    return t;
  };
  {
    const p = await visit('https://www.google.com/search?q=lgbtq+meaning', 'google topic');
    await topicPage(p, 'a Google search "lgbtq meaning"');
    check('T1: topic page text fits at font 20 and 110 % zoom', await fits(p));
    await p.close();
  }
  { const p = await visit('https://duckduckgo.com/?q=g4y+pride', 'ddg trick'); await topicPage(p, 'a DuckDuckGo search with a spelling trick ("g4y pride")'); await p.close(); }
  { const p = await visit('http://topic.kgtest.example/guide', 'topic page'); await topicPage(p, 'a page titled "Coming out as transgender"'); await p.close(); }
  { const p = await visit('https://glaad.org/', 'topic site'); await topicPage(p, 'a site on the topic list (glaad.org)'); await p.close(); }
  { const p = await visit('https://www.youtube.com/watch?v=aaaaaaaaaa1', 'yt watch topic', 2500); await topicPage(p, 'a YouTube watch page titled "Pride Month special"'); await p.close(); }
  {
    const p = await visit('https://www.youtube.com/results?search_query=volcano', 'yt results', 2500);
    const r = await p.evaluate(() => [...document.querySelectorAll('ytd-video-renderer')].map((e) => `${e.dataset.id}:${getComputedStyle(e).display !== 'none'}`));
    check('T1: in YouTube results, a title on the topic is hidden, the others show', r.join(',') === 'cccccccccc1:true,cccccccccc2:false,cccccccccc3:true', r.join(','));
    await p.close();
  }
  for (const [url, label] of [['http://chem.kgtest.example/', "Gay-Lussac's law"], ['http://books.kgtest.example/', 'Pride and Prejudice'], ['https://www.youtube.com/watch?v=bbbbbbbbbb1', 'a watch page "Pride and Prejudice"'], ['https://www.google.com/search?q=transformers+toys', 'a search "transformers toys"']]) {
    const p = await visit(url, label, 2500);
    const shown = await p.evaluate(() => getComputedStyle(document.documentElement).visibility !== 'hidden').catch(() => false);
    check(`T1: ordinary look-alike passes: ${label}`, reasonOf(p.url()) === null && shown, p.url());
    await p.close();
  }
  {
    const p = await visit('https://findahelpline.com/', 'helpline', 2500);
    check('T1: a helpline about the topic is never blocked', reasonOf(p.url()) === null && (await p.$eval('#ok', (e) => e.textContent).catch(() => '')) === 'Free, confidential support', p.url());
    await p.close();
  }

  // Helplines in every path (review M1): adult words in the body of a helpline page do not flag it; the same
  // body on another site does (so the check is active).
  {
    const a = await visit('https://findahelpline.com/words', 'helpline words', 3500);
    check('M1: a helpline page with adult words in its body is not blocked (content check path)', reasonOf(a.url()) === null && (await a.evaluate(() => getComputedStyle(document.documentElement).visibility)) !== 'hidden', a.url());
    await a.close();
    const b = await visit('http://words.kgtest.example/words', 'words control', 3500);
    check('M1 control: the same body on another site goes to the content block page', reasonOf(b.url()) === 'content', b.url());
    await b.close();
  }
  // Single-page sites (review S6, S7): a new address and title without a reload are checked again.
  {
    const p = await visit('http://spa.kgtest.example/', 'spa', 500);
    check('S6: a single-page site that moves to a topic title goes to the topic page', await waitReason(p, 'topic', 9000) === 'topic', p.url());
    await p.close();
    const y = await visit('https://www.youtube.com/', 'yt hashtag', 2500);
    await y.evaluate(() => { history.pushState({}, '', '/hashtag/gaypride'); document.title = '#gaypride - YouTube'; document.dispatchEvent(new Event('yt-navigate-finish')); });
    check('S7: a YouTube hashtag page reached in-page (#gaypride) goes to the topic page', await waitReason(y, 'topic', 6000) === 'topic', y.url());
    await y.close();
  }
  // N8: the helpline line also on the bedtime and paused pages.
  for (const r of ['bedtime', 'paused']) {
    const p = await visit(`${ORIGIN}/pages/blocked.html?reason=${r}`, `help ${r}`, 1200);
    const h = await p.evaluate(() => ({ shown: !document.getElementById('help').hidden, text: document.getElementById('help').textContent, below: document.getElementById('help').getBoundingClientRect().top >= document.getElementById('body').getBoundingClientRect().bottom }));
    check(`N8: the ${r} page shows "If you need help..." with findahelpline.com, under the words`, h.shown && h.below && h.text.startsWith('If you need help, you can always talk to someone:') && h.text.includes('findahelpline.com'), JSON.stringify(h));
    await p.close();
  }

  // A parent allows the page with the PIN (existing grant).
  {
    const p = await visit('http://topic.kgtest.example/guide', 'topic allow');
    await waitReason(p, 'topic');
    await p.click('#askBtn').catch(() => {});
    await p.type('#pin', PIN).catch(() => {});
    await p.click('#pinForm button[type=submit]').catch(() => {});
    await p.waitForSelector('#grants button', { timeout: 5000 }).catch(() => {});
    const label = await p.$eval('#grants button', (e) => e.textContent).catch(() => '');
    await Promise.all([p.waitForNavigation({ timeout: 8000 }).catch(() => {}), p.click('#grants button').catch(() => {})]);
    await sleep(2500);
    check('T1: a parent can allow the page with the PIN', /topic\.kgtest\.example/.test(label) && reasonOf(p.url()) === null && /topic\.kgtest\.example/.test(p.url()), `${label} -> ${p.url()}`);
    await p.close();
  }

  // ---- the parent page switch ----
  {
    const text = await parentPage();
    check('parent page: "Sexuality and gender identity topics" with its plain line, on', text.includes('Sexuality and gender identity topics') && text.includes('Helplines stay open.') && await parent.$eval('input[name="topics"]', (e) => e.checked));
    await parent.click('input[name="topics"]');
    await submit('form[action="/parent/save"] button[type=submit]');
    const r = await sync();
    check('parent switches the topic filter off: the extension follows', r && r.cfg && r.cfg.topics === false, JSON.stringify(r && r.cfg && r.cfg.topics));
    await sleep(500);
  }
  for (const [url, label] of [['https://www.google.com/search?q=lgbtq+meaning', 'the Google search'], ['https://glaad.org/', 'the topic site'], ['https://www.youtube.com/watch?v=aaaaaaaaaa1', 'the YouTube watch page']]) {
    const p = await visit(url, `off ${label}`, 2500);
    check(`switch off: ${label} opens`, reasonOf(p.url()) === null, p.url());
    await p.close();
  }
  {
    const p = await visit('https://www.youtube.com/results?search_query=volcano', 'off yt results', 2500);
    const n = await p.evaluate(() => [...document.querySelectorAll('ytd-video-renderer')].filter((e) => getComputedStyle(e).display !== 'none').length);
    check('switch off: every YouTube result shows', n === 3, String(n));
    await p.close();
  }

  // The report counts topic pages under their own reason (sent to the agent with each block page).
  {
    const text = await parentPage();
    check('parent report: blocked topic pages under "Sexuality and gender identity topics"', /Sexuality and gender identity topics\s*\n?\s*\d/.test(text) || (text.match(/Sexuality and gender identity topics/g) || []).length >= 2, (text.match(/Blocked pages by reason[\s\S]{0,300}/) || [''])[0].replace(/\s+/g, ' ').slice(0, 200));
  }

  check('no console errors', errors.length === 0, errors.slice(0, 5).join(' | '));
  exitCode = results.every((r) => r.ok) ? 0 : 1;
} catch (e) {
  console.error('FAIL (crash)', e);
} finally {
  await browser.close().catch(() => {});
  server.close(); tls.close();
  try { await fetch(`${AGENT}/__stop`); } catch { /* already stopped */ }
  agentProc.kill();
  rmSync(profile, { recursive: true, force: true });
  rmSync(certDir, { recursive: true, force: true });
}
const passed = results.filter((r) => r.ok).length;
console.log(`\n${passed}/${results.length} passed`);
process.exit(exitCode);
