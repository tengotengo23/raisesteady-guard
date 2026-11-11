// Store screenshots (1280x800) from the dev build in standalone mode, English, with the reduced default font.
//   node tools/build-ext.mjs --dev && node tools/store-screenshots.mjs <outdir>
import { mkdirSync, mkdtempSync, writeFileSync, readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { loadPuppeteer } from './puppeteer.mjs';
const KG = join(dirname(fileURLToPath(import.meta.url)), '..');
const OUT = process.argv[2] || join(KG, 'dist', 'store-screenshots');
mkdirSync(OUT, { recursive: true });
const EXT = join(KG, 'dist', 'chromium-dev');
const ID = JSON.parse(readFileSync(join(KG, 'shared', 'ext-ids.json'), 'utf8')).chromium.devId;
const O = `chrome-extension://${ID}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const profile = mkdtempSync(join(tmpdir(), 'kg-shots-'));
const browser = await loadPuppeteer().launch({ headless: true, pipe: true, enableExtensions: [EXT], userDataDir: profile,
  args: ['--no-first-run', '--no-default-browser-check', '--lang=en-US'] });
const msg = async (m) => { const p = await browser.newPage(); await p.goto(`${O}/pages/popup.html`); const r = await p.evaluate((x) => chrome.runtime.sendMessage(x), m); await p.close(); return r; };
try {
  await browser.waitForTarget((t) => t.type() === 'service_worker' && t.url().startsWith(O), { timeout: 15000 });
  await sleep(800);
  await msg({ type: 'kg:core:test', op: 'set-settings', settings: { ageBand: '9-12', bedtime: null } });
  await msg({ type: 'kg:core:settings-save', newPin: '2468', settings: { ageBand: '9-12', bedtime: null, focusMode: true } });
  await sleep(700);
  const shot = async (name, url, sel, zoom) => {
    const p = await browser.newPage();
    await p.setViewport({ width: 1280, height: 800, deviceScaleFactor: 1 });
    try { await p.goto(url, { waitUntil: 'load', timeout: 15000 }); } catch {}
    await sleep(1200);
    if (zoom) await p.evaluate((z) => { document.body.style.zoom = z; }, zoom);
    if (sel) await sel(p);
    if (zoom) await sleep(300);
    await p.screenshot({ path: join(OUT, name + '.png') });
    console.log(name, p.url().slice(0, 90));
    await p.close();
  };
  await shot('1-blocked-adult', `${O}/pages/blocked.html?reason=adult&site=example-adult-site.com`, null, 0.82);
  await shot('2-time-is-up', `${O}/pages/blocked.html?reason=time&site=youtube.com`, null, 0.82);
  await shot('3-bedtime', `${O}/pages/blocked.html?reason=bedtime&site=youtube.com`, null, 0.82);
  await shot('4-new-tab', `${O}/pages/newtab.html`);
  await shot('5-parent-page', `${O}/pages/options.html`, async (p) => {
    try { await p.type('#lockPin', '2468'); await p.keyboard.press('Enter'); await sleep(1500); } catch {}
  });
  // popup: a 360px-wide panel, shown large on a cream 1280x800 canvas with a caption
  await shot('6-popup', `${O}/pages/popup.html`, async (p) => {
    await p.evaluate(() => {
      const w = document.createElement('div');
      w.style.cssText = 'position:fixed;inset:0;background:#fbf3e4;display:flex;align-items:center;justify-content:center;gap:64px;z-index:99;font-family:system-ui,sans-serif';
      const card = document.createElement('div');
      card.style.cssText = 'background:#14152a;width:360px;border-radius:18px;overflow:hidden;box-shadow:0 20px 60px rgba(0,0,0,.35);zoom:1.15';
      const kids = [...document.body.childNodes];
      kids.forEach((k) => card.appendChild(k));
      const cap = document.createElement('div');
      cap.style.cssText = 'font-size:44px;font-weight:700;color:#2b2540;max-width:420px;line-height:1.2';
      cap.textContent = 'Time and settings, at a glance';
      w.append(card, cap); document.body.appendChild(w);
    });
  });
} finally { await browser.close(); }
