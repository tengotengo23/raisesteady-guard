#!/usr/bin/env node
// End-to-end: Chrome headless (new) with the unpacked dev build in standalone mode.
//   node tools/build-ext.mjs --dev && node tests/e2e/a-e2e.mjs
// Every host maps to a local server (--host-resolver-rules), so no real site is contacted.
import { createRequire } from 'node:module';
import { readFileSync, mkdirSync, writeFileSync, rmSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:http';
import { createServer as createHttpsServer } from 'node:https';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { loadPuppeteer } from '../../tools/puppeteer.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const KG = join(HERE, '..', '..');
const REPO = join(KG, '..');
const EXT = join(KG, 'dist', 'chromium-dev');
const FIX = join(HERE, 'fixtures-a');
const puppeteer = loadPuppeteer();
if (existsSync(join(REPO, 'orchestration', 'LOW_RAM'))) { console.log('LOW_RAM is set: skipping the Chrome run'); process.exit(2); }
if (!existsSync(join(EXT, 'manifest.json'))) { console.error('Build first: node tools/build-ext.mjs --dev'); process.exit(1); }

const DEV_ID = JSON.parse(readFileSync(join(KG, 'shared', 'ext-ids.json'), 'utf8')).chromium.devId;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const results = [];
const check = (name, ok, detail = '') => { results.push({ name, ok: !!ok }); console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ` (${detail})` : ''}`); };

// ---- local web server for every host ----
const seen = [];
const server = createServer((req, res) => {
  const host = String(req.headers.host || '').split(':')[0];
  seen.push({ host, url: req.url, headers: req.headers });
  const send = (file, type = 'text/html; charset=utf-8') => { res.writeHead(200, { 'content-type': type }); res.end(readFileSync(join(FIX, file))); };
  if (req.url === '/favicon.ico') { res.writeHead(404); return res.end(); }
  // Thumbnails of the built-in videos (P2-161: the new tab shows them while the feeds do not answer): a 1x1 PNG.
  if (host === 'i.ytimg.com') { res.writeHead(200, { 'content-type': 'image/png' }); return res.end(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64')); }
  if (host === 'rta.kgtest.example') return send('rta.html');
  if (host === 'health.kgtest.example') return send('health.html');
  if (req.url.startsWith('/words')) return send('words.html');
  send('plain.html');
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const PORT = server.address().port;
// HSTS-preloaded hosts (bing.com, youtube.com) are always https: a second server with a throwaway self-signed cert.
const certDir = join(tmpdir(), `kg-e2e-a-cert-${process.pid}`);
mkdirSync(certDir, { recursive: true });
execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-subj', '/CN=kgtest', '-keyout', join(certDir, 'k.pem'), '-out', join(certDir, 'c.pem')], { stdio: 'ignore' });
const tls = createHttpsServer({ key: readFileSync(join(certDir, 'k.pem')), cert: readFileSync(join(certDir, 'c.pem')) }, (req, res) => server.emit('request', req, res));
await new Promise((r) => tls.listen(0, '127.0.0.1', r));
const TLS_PORT = tls.address().port;
const HSTS = ['www.bing.com', 'www.youtube.com', 'en.wikipedia.org', 'www.bbc.co.uk', 'i.ytimg.com'];

// ---- profile with large font settings (Chrome default 20, fixed 17) and 110 % zoom ----
const profile = join(tmpdir(), `kg-e2e-a-${process.pid}`);
mkdirSync(join(profile, 'Default'), { recursive: true });
writeFileSync(join(profile, 'Default', 'Preferences'), JSON.stringify({
  webkit: { webprefs: { default_font_size: 20, default_fixed_font_size: 17 } },
  partition: { default_zoom_level: { x: Math.log(1.1) / Math.log(1.2) } },
}));

const browser = await puppeteer.launch({
  headless: true,
  pipe: true,
  enableExtensions: [EXT],
  userDataDir: profile,
  args: [
    `--host-resolver-rules=${HSTS.map((h) => `MAP ${h} 127.0.0.1:${TLS_PORT}`).join(', ')}, MAP * 127.0.0.1:${PORT}, EXCLUDE localhost`,
    '--ignore-certificate-errors',
    '--disable-features=HttpsUpgrades,HttpsFirstBalancedModeAutoEnable',
    '--no-first-run', '--no-default-browser-check',
  ],
});

let exitCode = 1;
try {
  const swTarget = await browser.waitForTarget((t) => t.type() === 'service_worker' && t.url().startsWith(`chrome-extension://${DEV_ID}/`), { timeout: 15000 });
  check('extension loads with the stable dev id', !!swTarget, DEV_ID);
  const ORIGIN = `chrome-extension://${DEV_ID}`;
  await sleep(800);

  const hook = async (msg) => {
    const p = await browser.newPage();
    await p.goto(`${ORIGIN}/pages/popup.html`);
    const r = await p.evaluate((m) => chrome.runtime.sendMessage(m), msg);
    await p.close();
    return r;
  };
  const visit = async (url, waitMs = 600) => {
    const p = await browser.newPage();
    try { await p.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 }); } catch (e) { /* the block redirect can abort the first load */ }
    await sleep(waitMs);
    const final = p.url();
    return { p, final };
  };
  const reasonOf = (u) => (String(u).startsWith(`${ORIGIN}/pages/blocked.html?`) ? new URLSearchParams(String(u).split('?')[1]).get('reason') : null);

  // 1. A listed adult domain
  const adult = JSON.parse(readFileSync(join(EXT, 'rules', 'adult.json'), 'utf8')).find((r) => r.action.type === 'redirect');
  const adultHost = adult && adult.condition.requestDomains && adult.condition.requestDomains[Math.floor(adult.condition.requestDomains.length / 2)];
  if (!adultHost) check('adult ruleset has domains', false, 'rules/adult.json is empty');
  else {
    const { p, final } = await visit(`http://${adultHost}/`);
    check('listed adult domain ends on the block page (adult)', reasonOf(final) === 'adult', `${adultHost} -> ${final}`);
    await p.close();
  }

  // 2. RTA-labelled page, 3. health page passes
  {
    const { p, final } = await visit('http://rta.kgtest.example/', 1500);
    check('RTA-labelled page is blocked as content', reasonOf(final) === 'content', final);
    const site = await p.evaluate(() => document.querySelector('#site')?.textContent || '').catch(() => '');
    check('block page names the site', site.includes('rta.kgtest.example'), site);
    await p.close();
    const other = await visit('http://rta.kgtest.example/another-page', 600);
    check('a labelled host stays blocked for the day (other pages too)', reasonOf(other.final) === 'content', other.final);
    await other.p.close();
  }
  // 2b. Word evidence blocks that page only; a mixed host (Wikipedia) and a normal host stay open elsewhere
  for (const base of ['https://en.wikipedia.org', 'http://words.kgtest.example']) {
    const a = await visit(`${base}/words-page`, 1500);
    check(`word evidence blocks the page (${base})`, reasonOf(a.final) === 'content', a.final);
    await a.p.close();
    const b = await visit(`${base}/wiki/Cats`, 1200);
    check(`the rest of the host stays open (${base})`, b.final.startsWith(`${base}/wiki/Cats`), b.final);
    await b.p.close();
    const again = await visit(`${base}/words-page?again=1`, 300);
    check(`the flagged page stays blocked for the day (${base})`, reasonOf(again.final) === 'content', again.final);
    await again.p.close();
  }
  {
    const { p, final } = await visit('http://health.kgtest.example/', 4000);
    check('sex-education page passes (also after the 3 s re-check)', final.startsWith('http://health.kgtest.example/'), final);
    await p.close();
  }

  // 4. Google search gets safe=active (and no redirect loop)
  {
    const { p, final } = await visit('http://www.google.com/search?q=cats', 800);
    const u = new URL(final);
    check('Google search gets safe=active', u.searchParams.get('safe') === 'active' && u.hostname === 'www.google.com', final);
    const hits = seen.filter((s) => s.host === 'www.google.com' && s.url.startsWith('/search'));
    check('server saw one safe search request, no loop', hits.length >= 1 && hits.length <= 2 && hits.every((h) => h.url.includes('safe=active')), `${hits.length} requests`);
    await p.close();
  }
  {
    const { p } = await visit('https://www.bing.com/search?q=cats', 600);
    check('Bing search gets adlt=strict', new URL(p.url()).searchParams.get('adlt') === 'strict', p.url());
    await p.close();
  }

  // 5. YouTube gets the Restricted Mode header (band 9-12: Strict)
  {
    const { p } = await visit('https://www.youtube.com/', 600);
    const yt = seen.filter((s) => s.host === 'www.youtube.com' && s.url === '/').pop();
    check('YouTube-Restrict: Strict header sent', yt && yt.headers['youtube-restrict'] === 'Strict', yt ? String(yt.headers['youtube-restrict']) : 'no request');
    await p.close();
  }

  // 6. Time limit: an open tab is redirected, new pages blocked, learning sites still open
  {
    const { p: open } = await visit('http://news.kgtest.example/', 300);
    const r = await hook({ type: 'kg:core:test', op: 'set-usage', browseSec: 120 * 60 }); // band 9-12: exactly the 120-minute limit
    check('test hook sets usage (dev build only)', r && r.ok && r.timeOver === true, JSON.stringify(r && { timeOver: r.timeOver }));
    await sleep(800);
    check('open tab moves to the time block page', reasonOf(open.url()) === 'time', open.url());
    await open.close();
    const { p, final } = await visit('http://other.kgtest.example/page', 600);
    check('new page at the limit: time block page', reasonOf(final) === 'time', final);
    await sleep(600);
    const tiles = await p.$$eval('#tiles a', (as) => as.map((a) => a.href)).catch(() => []);
    const learnList = JSON.parse(readFileSync(join(EXT, 'data', 'learning-domains.json'), 'utf8')).domains;
    const isLearn = (u) => { const x = new URL(u); const h = x.hostname.replace(/^www\./, ''); return learnList.some((d) => (d.includes('/') ? (h === d.split('/')[0] || h.endsWith('.' + d.split('/')[0])) && x.pathname.startsWith('/' + d.split('/').slice(1).join('/')) : h === d || h.endsWith('.' + d))); };
    check('time block page suggests only learning sites', tiles.length > 0 && tiles.every(isLearn), `${tiles.length}: ${tiles.join(' ')}`);
    await p.close();
    const bite = await visit('https://www.bbc.co.uk/bitesize', 600);
    check('a learning path (BBC Bitesize) opens when time is up', bite.final.startsWith('https://www.bbc.co.uk/bitesize'), bite.final);
    await bite.p.close();
    const ipl = await visit('https://www.bbc.co.uk/iplayer', 600);
    check('the rest of that host is blocked when time is up', reasonOf(ipl.final) === 'time', ipl.final);
    await ipl.p.close();
    const learn = await visit('http://www.khanacademy.org/', 600);
    check('learning site still opens when time is up', learn.final.startsWith('http://www.khanacademy.org/'), learn.final);
    await learn.p.close();
  }

  // 7. Ask a parent: set a PIN (first setup), then add 15 minutes from the block page
  {
    const opt = await browser.newPage();
    await opt.goto(`${ORIGIN}/pages/options.html`);
    const saved = await opt.evaluate(() => chrome.runtime.sendMessage({ type: 'kg:core:settings-save', newPin: '2468', settings: { ageBand: '9-12', limits: {}, focusMode: true, adblock: true, allow: [], block: [] } }));
    check('first setup saves a PIN', saved && saved.ok && saved.settings && saved.settings.hasPin === true && !('pin' in saved.settings), JSON.stringify(saved && saved.ok));
    await opt.close();
    const { p } = await visit('http://news.kgtest.example/again', 800);
    await p.waitForSelector('#askBtn', { visible: true, timeout: 5000 });
    await p.click('#askBtn');
    await p.type('#pin', '1111');
    await p.click('#pinForm button[type=submit]');
    await p.waitForSelector('#pinMsg:not([hidden])', { timeout: 5000 });
    const wrong = await p.$eval('#pinMsg', (e) => e.textContent);
    check('wrong PIN shows a calm message under the field', /didn't work/.test(wrong), wrong);
    await p.$eval('#pin', (e) => { e.value = ''; });
    await p.type('#pin', '2468');
    await p.click('#pinForm button[type=submit]');
    await p.waitForSelector('#grants button', { visible: true, timeout: 5000 });
    const labels = await p.$$eval('#grants button', (bs) => bs.map((b) => b.textContent));
    check('right PIN offers 15, 30 or 60 minutes', labels.length === 3 && labels[0].includes('15'), labels.join(' | '));
    await p.click('#grants button');
    await sleep(1800);
    check('after "Add 15 minutes" the page opens again', p.url().startsWith('http://news.kgtest.example/again'), p.url());
    await p.close();
    await hook({ type: 'kg:core:test', op: 'set-usage', browseSec: 0 });
  }

  // 8. Layout: new tab and block pages at 360 and 1280 px, font 20 / fixed 17, 110 % zoom, both motion modes
  const pages = [['new tab', `${ORIGIN}/pages/newtab.html`], ['block page (adult)', `${ORIGIN}/pages/blocked.html?reason=adult&site=example.com`], ['block page (time)', `${ORIGIN}/pages/blocked.html?reason=time`], ['block page (bedtime)', `${ORIGIN}/pages/blocked.html?reason=bedtime`], ['block page (curated)', `${ORIGIN}/pages/blocked.html?reason=curated`], ['popup', `${ORIGIN}/pages/popup.html`], ['options', `${ORIGIN}/pages/options.html`]];
  for (const [label, url] of pages) {
    for (const width of [360, 1280]) {
      for (const motion of ['reduce', 'no-preference']) {
        const p = await browser.newPage();
        const errors = [];
        p.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
        p.on('pageerror', (e) => errors.push(String(e)));
        await p.setViewport({ width, height: 900 });
        await p.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: motion }]);
        await p.goto(url, { waitUntil: 'networkidle0' });
        if (label === 'block page (adult)') { await p.click('#askBtn'); }
        await sleep(400);
        const m = await p.evaluate(() => {
          const vis = (e) => { const r = e.getBoundingClientRect(); const cs = getComputedStyle(e); return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden'; };
          const small = [];
          for (const e of document.querySelectorAll('a, button, input:not([type=hidden]), select, textarea, [role=button]')) {
            const t = (e.matches('input[type=checkbox], input[type=radio]') && e.closest('label')) || e;
            if (!vis(t)) continue;
            const r = t.getBoundingClientRect();
            if (r.height < 44 - 0.5 || r.width < 44 - 0.5) small.push(`${t.tagName.toLowerCase()}#${t.id || ''}.${t.className || ''} ${Math.round(r.width)}x${Math.round(r.height)}`);
          }
          const overflow = [];
          for (const e of document.querySelectorAll('.kg-btn, .kg-tile, .kg-card, .kg-choice span, h1, h2, p')) if (vis(e) && e.scrollWidth > e.clientWidth + 1) overflow.push(e.id || e.className || e.tagName);
          return {
            sideScroll: document.documentElement.scrollWidth > window.innerWidth + 1,
            small, overflow,
            bodyFont: getComputedStyle(document.body).fontSize,
            dpr: window.devicePixelRatio, inner: window.innerWidth,
            hasTitle: !!document.querySelector('h1') && document.querySelector('h1').textContent.trim().length > 0,
          };
        });
        const tag = `${label} @${width} ${motion}`;
        const ok = !errors.length && !m.sideScroll && !m.small.length && !m.overflow.length && m.bodyFont === '20px' && m.hasTitle;
        check(`layout ${tag}`, ok, ok ? `font ${m.bodyFont}, dpr ${m.dpr}, inner ${m.inner}` : JSON.stringify({ errors, ...m }));
        await p.close();
      }
    }
  }

  // 8b. Curated reason (ages 3-5 go there): calm copy, no "allow this site"; privacy line in popup and options
  {
    const p = await browser.newPage();
    await p.goto(`${ORIGIN}/pages/blocked.html?reason=curated&site=www.youtube.com`, { waitUntil: 'networkidle0' });
    const v = await p.evaluate(() => ({ t: document.querySelector('#title').textContent, ask: !document.querySelector('#ask').hidden }));
    check('curated block page: copy and no allow option', v.t === "This video isn't on your list yet." && !v.ask, JSON.stringify(v));
    for (const pg of ['popup', 'options']) {
      await p.goto(`${ORIGIN}/pages/${pg}.html`, { waitUntil: 'networkidle0' });
      const txt = await p.evaluate(() => document.body.innerText);
      check(`${pg} shows the privacy line`, txt.includes('Nothing is sent to RaiseSteady.'), pg);
    }
    await p.close();
  }

  // 9. Bedtime (last: it replaces the settings)
  {
    const now = new Date();
    const hm = (d) => `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    const r = await hook({ type: 'kg:core:test', op: 'set-settings', settings: { ageBand: '9-12', bedtime: { start: hm(new Date(now - 3600e3)), end: hm(new Date(+now + 3600e3)) } } });
    check('bedtime set through the test hook', r && r.bedtime === true, JSON.stringify(r && r.bedtime));
    const { p, final } = await visit('http://www.khanacademy.org/', 600);
    check('bedtime blocks every site, learning ones too', reasonOf(final) === 'bedtime', final);
    await p.close();
  }
  exitCode = results.every((r) => r.ok) ? 0 : 1;
} catch (e) {
  console.error('E2E crashed:', e);
  check('run completed', false, String(e && e.message));
} finally {
  await browser.close().catch(() => {});
  server.close(); tls.close();
  rmSync(certDir, { recursive: true, force: true });
  rmSync(profile, { recursive: true, force: true });
}
const pass = results.filter((r) => r.ok).length;
console.log(`\n${pass}/${results.length} passed`);
process.exit(exitCode);
