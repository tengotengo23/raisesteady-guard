#!/usr/bin/env node
// End-to-end, managed mode (P2-160): Chrome headless with the unpacked dev build talking to the real Go
// agent API and parent page (TestServeForE2E, a test server on 127.0.0.1:47616, PIN 2468).
//   node tools/build-ext.mjs --dev && node tests/e2e/c-managed.mjs
// Covers: "Ask a parent" on the block page and on the time-is-up page (request, then the parent answers on
// the parent page), the paused page (and ending it with the PIN), the homework page, search words in the
// report, the popup lines, no console errors, and text in boxes at Chrome font 20 / fixed 17 and 110 % zoom.
// Every web host maps to a local server (--host-resolver-rules), so no real site is contacted.
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
const puppeteer = loadPuppeteer();
if (existsSync(join(REPO, 'orchestration', 'LOW_RAM'))) { console.log('LOW_RAM is set: skipping the Chrome run'); process.exit(2); }
if (!existsSync(join(EXT, 'manifest.json'))) { console.error('Build first: node tools/build-ext.mjs --dev'); process.exit(1); }

const DEV_ID = JSON.parse(readFileSync(join(KG, 'shared', 'ext-ids.json'), 'utf8')).chromium.devId;
const ORIGIN = `chrome-extension://${DEV_ID}`;
const AGENT_PORT = 47616;
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
const agentState = async () => (await fetch(`${AGENT}/v1/state`, { headers: { 'X-KG-Token': TOKEN } })).json();
let up = false;
for (let i = 0; i < 240 && !up; i++) { try { up = (await fetch(`${AGENT}/parent`)).ok; } catch { await sleep(500); } }
if (!up) { console.error('the agent test server did not start'); agentProc.kill(); process.exit(1); }

// ---- local web server for every host (plain page) ----
const server = createServer((req, res) => {
  if (req.url === '/favicon.ico') { res.writeHead(204); return res.end(); }
  res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
  res.end(`<!doctype html><title>ok</title><h1 id="ok">${String(req.headers.host || '').replace(/[^a-z0-9.:-]/gi, '')}</h1>`);
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const certDir = join(tmpdir(), `kg-e2e-c-cert-${process.pid}`);
mkdirSync(certDir, { recursive: true });
execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-subj', '/CN=kgtest', '-keyout', join(certDir, 'k.pem'), '-out', join(certDir, 'c.pem')], { stdio: 'ignore' });
const tls = createHttpsServer({ key: readFileSync(join(certDir, 'k.pem')), cert: readFileSync(join(certDir, 'c.pem')) }, (req, res) => server.emit('request', req, res));
await new Promise((r) => tls.listen(0, '127.0.0.1', r));
const HTTPS_HOSTS = ['www.google.com', 'www.khanacademy.org', 'duckduckgo.com'];
const HTTP_HOSTS = ['plain.kgtest.example', 'blocked.kgtest.example', 'other.kgtest.example'];

const profile = join(tmpdir(), `kg-e2e-c-${process.pid}`);
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

// Console errors from our pages, the worker and the parent page.
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
  const visit = async (url, label, waitMs = 1200) => {
    const p = await browser.newPage();
    watch(p, label);
    try { await p.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 }); } catch { /* the block redirect can abort the first load */ }
    await sleep(waitMs);
    return p;
  };
  const reasonOf = (u) => (String(u).startsWith(`${ORIGIN}/pages/blocked.html?`) ? new URLSearchParams(String(u).split('?')[1]).get('reason') : null);
  const fits = (p) => p.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1
    && [...document.querySelectorAll('button, input, label, p, h1')].every((e) => e.scrollWidth <= e.clientWidth + 2 || getComputedStyle(e).overflowX !== 'visible'));

  // The parent page in its own tab (a parent signing in).
  const parent = await browser.newPage();
  watch(parent, 'parent page');
  await parent.setViewport({ width: 1100, height: 900 });
  const submit = (sel) => Promise.all([parent.waitForNavigation({ waitUntil: 'load' }), parent.click(sel)]);
  const parentPage = async () => {
    await parent.goto(`${AGENT}/parent?lang=en`, { waitUntil: 'load' });
    if (await parent.$('#pin')) { await parent.type('#pin', PIN); await submit('form[action="/parent/login"] button[type=submit]'); }
    return parent.evaluate(() => document.body.innerText);
  };

  // 0. Managed mode through the dev test hook (as the Windows policy would set it).
  const r0 = await hook({ type: 'kg:core:test', op: 'set-managed', managed: { agentToken: TOKEN, agentPort: AGENT_PORT } });
  const r1 = await sync();
  check('managed mode: the extension reaches the agent', r1 && r1.agentOnline === true && r0 && r0.cfg && r0.cfg.standalone === false, JSON.stringify({ online: r1 && r1.agentOnline, standalone: r0 && r0.cfg && r0.cfg.standalone }));

  // 1. Popup: the honest line about searches.
  {
    const p = await visit(`${ORIGIN}/pages/popup.html`, 'popup', 800);
    const line = await p.$eval('#honestSearches', (e) => (e.hidden ? '' : e.textContent));
    check('popup: "Your parent can see the searches you make"', line === 'Your parent can see the searches you make.', line);
    await p.close();
  }

  // 2. Searches: a Google and a DuckDuckGo result page, then the parent report.
  {
    const a = await visit('https://www.google.com/search?q=How+Do+Volcanoes+Work', 'google', 800);
    const b = await visit('https://duckduckgo.com/?q=Fractions+for+kids', 'ddg', 800);
    await a.close(); await b.close();
    await sleep(500);
    await sync();
    const text = await parentPage();
    check('searches: shown in the parent report for today', text.includes('how do volcanoes work') && text.includes('fractions for kids'), text.match(/Searches on [^\n]*/)?.[0] || '');
  }

  // 3. Block page "Ask a parent" (a site the parent blocked), then "Allow this site" on the parent page.
  {
    const p = await visit('http://blocked.kgtest.example/', 'blocked page');
    check('blocked site ends on the block page (parent)', reasonOf(p.url()) === 'parent', p.url());
    const shown = await p.$eval('#req', (e) => !e.hidden).catch(() => false);
    const label = await p.$eval('#reqBtn', (e) => e.textContent).catch(() => '');
    check('block page: "Ask a parent" with a note box under the PIN box', shown && label === 'Ask a parent', label);
    const order = await p.evaluate(() => !!(document.getElementById('askPanel').compareDocumentPosition(document.getElementById('req')) & Node.DOCUMENT_POSITION_FOLLOWING));
    check('block page: the request comes after the PIN box', order);
    await p.type('#reqNote', 'For my science project');
    await p.click('#reqBtn');
    await p.waitForSelector('#reqMsg:not([hidden])', { timeout: 5000 }).catch(() => {});
    const msg = await p.$eval('#reqMsg', (e) => e.textContent).catch(() => '');
    check('block page: "Asked. Your parent will see it on the parent page."', msg === 'Asked. Your parent will see it on the parent page.', msg);
    const below = await p.evaluate(() => document.getElementById('reqMsg').getBoundingClientRect().top >= document.getElementById('reqBtn').getBoundingClientRect().bottom);
    check('block page: the answer shows under the button', below);
    check('block page: text fits at font 20 and 110 % zoom', await fits(p));
    await p.close();

    const text = await parentPage();
    const iReq = text.indexOf('Requests from your child'), iPause = text.indexOf('Pause the internet');
    await parent.setViewport({ width: 390, height: 900 });
    check('parent page: no side scroll on a phone-width window at font 20 and 110 % zoom', await fits(parent));
    await parent.setViewport({ width: 1100, height: 900 });
    check('parent page: the request comes first, with its note', iReq >= 0 && iReq < iPause && text.includes('asks you to allow blocked.kgtest.example') && text.includes('For my science project'), text.slice(iReq, iReq + 160).replace(/\s+/g, ' '));
    await submit('form[action="/parent/request"] button[value="allow"]');
    check('parent page: "Allow this site" answers it', parent.url().includes('ok=req_done'), parent.url());
    await sync();
    const q = await visit('http://blocked.kgtest.example/', 'allowed site');
    check('the allowed site now opens', reasonOf(q.url()) === null && q.url().startsWith('http://blocked.kgtest.example'), q.url());
    await q.close();
  }

  // 4. Time-is-up page: "Ask a parent" for more time, then "15 min".
  {
    await hook({ type: 'kg:core:test', op: 'set-usage', browseSec: 720 * 60 + 5 });
    const p = await visit('http://plain.kgtest.example/', 'time page');
    check('time used up: the time-is-up page', reasonOf(p.url()) === 'time', p.url());
    await p.click('#reqBtn');
    await p.waitForSelector('#reqMsg:not([hidden])', { timeout: 5000 }).catch(() => {});
    check('time page: the request is recorded', (await p.$eval('#reqMsg', (e) => e.textContent).catch(() => '')) === 'Asked. Your parent will see it on the parent page.');
    await p.close();
    const text = await parentPage();
    check('parent page: the time request with 15 and 30 minutes', text.includes('asks for more time today') && !!(await parent.$('button[value="15"]')) && !!(await parent.$('button[value="30"]')));
    await submit('form[action="/parent/request"] button[value="15"]');
    const st = await agentState();
    check('parent page: "15 min" adds 15 minutes', st.extraMin === 15, `extraMin ${st.extraMin}`);
    await hook({ type: 'kg:core:test', op: 'set-usage', browseSec: 0, videoSec: 0 });
  }

  // 5. Pause: the parent pauses for 30 min; every site shows the paused page; the PIN there ends it.
  {
    await parentPage();
    await submit('form[action="/parent/pause"] button[value="30"]');
    const st = await agentState();
    check('parent page: "30 min" pauses the internet', st.paused === true && /^\d\d:\d\d$/.test(st.pausedUntilHM || ''), st.pausedUntilHM);
    await sync();
    const pop = await visit(`${ORIGIN}/pages/popup.html`, 'popup', 800);
    const line = await pop.$eval('#now', (e) => (e.hidden ? '' : e.textContent));
    check('popup: shows the pause', line === `Paused by your parent until ${st.pausedUntilHM}.`, line);
    await pop.close();
    const p = await visit('https://www.khanacademy.org/', 'paused page');
    check('paused: even a learning site shows the paused page', reasonOf(p.url()) === 'paused', p.url());
    const title = await p.$eval('#title', (e) => e.textContent).catch(() => '');
    check('paused page: "Paused by your parent until HH:MM"', title === `Paused by your parent until ${st.pausedUntilHM}.`, title);
    check('paused page: no request button, the PIN box is there', (await p.$eval('#req', (e) => e.hidden)) && !(await p.$eval('#askBtn', (e) => e.hidden)));
    check('paused page: text fits at font 20 and 110 % zoom', await fits(p));
    await p.click('#askBtn');
    await p.type('#pin', PIN);
    await p.click('#pinForm button[type=submit]');
    await p.waitForSelector('#grants button', { timeout: 5000 }).catch(() => {});
    const btn = await p.$eval('#grants button', (e) => e.textContent).catch(() => '');
    check('paused page: the PIN offers "End the pause"', btn === 'End the pause', btn);
    await p.click('#grants button');
    await p.waitForSelector('#grantMsg:not([hidden])', { timeout: 5000 }).catch(() => {});
    const done = await p.$eval('#grantMsg', (e) => e.textContent).catch(() => '');
    const after = await agentState();
    check('paused page: the pause ends', done === 'Done. The internet is back.' && after.paused === false, done);
    await p.close();
    await sync();
    const q = await visit('http://plain.kgtest.example/', 'after pause');
    check('after the pause sites open again', reasonOf(q.url()) === null, q.url());
    await q.close();
  }

  // 6. Homework time: the parent sets a window around now (a Wednesday on the agent clock).
  {
    await parentPage();
    const now = new Date((await agentState()).now);
    const mins = Number((await agentState()).now.slice(11, 13)) * 60 + Number((await agentState()).now.slice(14, 16));
    const f = (m) => `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
    const start = f(Math.max(0, mins - 60)), end = f(Math.min(23 * 60 + 59, mins + 60));
    await parent.click('input[name="homeworkOn"]');
    await parent.$eval('#hwStart', (e, v) => { e.value = v; }, start);
    await parent.$eval('#hwEnd', (e, v) => { e.value = v; }, end);
    await submit('form[action="/parent/save"] button[type=submit]');
    const st = await agentState();
    check('parent page: homework time saved and active', parent.url().includes('ok=saved') && st.homework === true && st.homeworkEnd === end, `${start}-${end} ${now.toISOString()}`);
    await sync();
    const p = await visit('http://other.kgtest.example/', 'homework page');
    check('homework time: a site that is not for learning shows the homework page', reasonOf(p.url()) === 'homework', p.url());
    const title = await p.$eval('#title', (e) => e.textContent).catch(() => '');
    check('homework page: "Homework time until HH:MM"', title === `Homework time until ${end}.`, title);
    check('homework page: "Ask a parent" for this site', !(await p.$eval('#req', (e) => e.hidden)));
    const tiles = await p.$$eval('#tiles a', (l) => l.length).catch(() => 0);
    check('homework page: learning sites to start with', tiles > 0, `${tiles} tiles`);
    check('homework page: text fits at font 20 and 110 % zoom', await fits(p));
    await p.close();
    const q = await visit('https://www.khanacademy.org/', 'learning in homework time');
    check('homework time: a learning site opens', reasonOf(q.url()) === null, q.url());
    await q.close();
    const a = await visit('http://blocked.kgtest.example/', 'allowed in homework time');
    check('homework time: a site the parent allowed opens', reasonOf(a.url()) === null, a.url());
    await a.close();
  }

  // 7. No console errors on the parent page, the popup, the block pages and the worker.
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
