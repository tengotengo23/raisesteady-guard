// Firefox: the real toolbar popup opens at its own width and stays there (P2-271). Seen on Fedora: the popup
// sometimes opened as a thin column of text. Firefox sizes a popup to its content, so the popup sets its own
// width (22rem); a narrow-screen rule that matched at 22rem made the panel content-sized again (569 px wide
// around a 352 px page, measured 23 Nov). This opens the real popup with action.openPopup() and measures it.
//   node tools/build-ext.mjs --dev && node tests/e2e/f-firefox-popup.mjs [firefox program]
// PUPPETEER_FROM: a package.json whose node_modules has puppeteer (default: kidguard's own, else nexodyne-server).
// Test-only settings, in puppeteer's throwaway profile: popups may open without a click, the add-on gets its host
// permissions at install (the block page redirect needs them), and remote system access, because WebDriver BiDi
// neither opens nor reads moz-extension pages otherwise. The test reaches an extension page through the
// web-accessible block page: a site on the adult list redirects there before any network request.
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPuppeteer } from '../../tools/puppeteer.mjs';

const KG = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const DIR = join(KG, 'dist', 'firefox-dev');
const puppeteer = process.env.PUPPETEER_FROM ? createRequire(process.env.PUPPETEER_FROM)('puppeteer') : loadPuppeteer();
const [exe] = process.argv.slice(2);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const id = JSON.parse(readFileSync(join(DIR, 'manifest.json'), 'utf8')).browser_specific_settings.gecko.id;
const UUID = '5d1f3e6a-2b7c-4c1e-9a8d-6f0e2b3c4d5e';
const rules = JSON.parse(readFileSync(join(DIR, 'rules', 'adult.json'), 'utf8'));
const host = rules.find((r) => r.action.type === 'redirect' && r.condition.requestDomains).condition.requestDomains[0];

let pass = 0, fail = 0;
const check = (name, ok, info = '') => { if (ok) pass++; else fail++; console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${info ? ` (${info})` : ''}`); };

const browser = await puppeteer.launch({
  browser: 'firefox',
  executablePath: exe || undefined,
  headless: true,
  env: { ...process.env, MOZ_REMOTE_ALLOW_SYSTEM_ACCESS: '1' },
  args: ['--remote-allow-system-access'],
  extraPrefsFirefox: {
    'extensions.openPopupWithoutUserGesture.enabled': true,
    'extensions.originControls.grantByDefault': true,
    'extensions.webextensions.uuids': JSON.stringify({ [id]: UUID }),
  },
});
let code = 1;
try {
  await browser.installExtension(DIR);
  await sleep(3000); // the static rulesets load
  const page = await browser.newPage();
  try { await page.goto(`http://${host}/`, { timeout: 8000 }); } catch { /* the redirect to the block page aborts the load */ }
  const where = await page.evaluate(() => location.href).catch((e) => String(e).slice(0, 120));
  check('a site on the adult list lands on the block page (an extension page)', String(where).startsWith(`moz-extension://${UUID}/pages/blocked.html`), where);
  await page.evaluate(() => browser.action.openPopup());
  const measure = () => page.evaluate(() => browser.extension.getViews({ type: 'popup' }).map((v) => ({
    view: v.innerWidth,
    html: Math.round(v.document.documentElement.getBoundingClientRect().width),
    own: Math.round(22 * parseFloat(v.getComputedStyle(v.document.documentElement).fontSize)),
    side: v.document.documentElement.scrollWidth > v.innerWidth + 1,
  })));
  await sleep(700);
  const m1 = await measure();
  await sleep(1500);
  const m2 = await measure();
  check('the toolbar popup opens', m1.length === 1 && m2.length === 1, JSON.stringify([m1, m2]));
  const ok = (m) => m && m.html === m.own && Math.abs(m.view - m.own) <= 2 && !m.side;
  check('the popup is its own 22rem wide, the panel too (no thin column, no empty band), and it stays', ok(m1[0]) && ok(m2[0]), JSON.stringify([m1[0], m2[0]]));
  code = fail ? 1 : 0;
} catch (e) {
  check('test ran', false, String(e).slice(0, 300));
} finally {
  await browser.close();
}
console.log(`\n${pass}/${pass + fail} passed`);
process.exit(code);
