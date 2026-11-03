// The parent page's forms in a real browser (P2-157). A browser adds its own Origin and Sec-Fetch headers to a form
// POST, which the Go tests (httptest) do not: with "Referrer-Policy: no-referrer" Firefox sent "Origin: null" and the
// agent answered "forbidden" to the right PIN. This logs in, saves the settings and logs out, as a parent would.
//   cd kidguard/agent && KG_SERVE_SECONDS=180 go test ./internal/api -run TestServeForBrowser &     (PIN 2468; GET /__stop ends it)
// or against the installed agent: KG_PIN=<its PIN>.
//   node kidguard/tests/e2e/parent-login.mjs chrome|firefox [browser program]
// PUPPETEER_FROM: a package.json whose node_modules has puppeteer (default: kidguard's own).
import { createRequire } from 'module';

const require = createRequire(process.env.PUPPETEER_FROM || import.meta.url);
const puppeteer = require('puppeteer');
const [kind = 'chrome', exe] = process.argv.slice(2);
const BASE = process.env.KG_URL || 'http://127.0.0.1:47615';
const PIN = process.env.KG_PIN || '2468';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const browser = await puppeteer.launch({
  browser: kind === 'firefox' ? 'firefox' : 'chrome',
  executablePath: exe || undefined,
  headless: true,
  args: kind === 'firefox' ? [] : ['--no-sandbox'],
});
const fails = [];
try {
  const page = await browser.newPage();
  const posts = [];
  page.on('response', (r) => {
    if (r.request().method() === 'POST') posts.push(new URL(r.url()).pathname + ' ' + r.status());
  });
  for (let i = 0; ; i++) {   // the test server may still be starting
    try {
      await page.goto(BASE + '/parent?lang=en', { waitUntil: 'load' });
      break;
    } catch (e) {
      if (i > 120) throw e;
      await sleep(500);
    }
  }
  const text = async () => (await page.evaluate(() => document.body.innerText)).replace(/\s+/g, ' ').slice(0, 120);
  const submit = (action) => Promise.all([
    page.waitForNavigation({ waitUntil: 'load' }),
    page.click(`form[action="${action}"] button[type=submit]`),
  ]);
  await page.type('#pin', PIN);
  await submit('/parent/login');
  if (!(await page.$('form[action="/parent/logout"]'))) {
    fails.push('login: ' + (await text()));
  } else {
    await submit('/parent/save');
    if (!page.url().includes('ok=saved')) fails.push('save: ' + (await text()));
    await submit('/parent/logout');
    if (!(await page.$('#pin'))) fails.push('logout: ' + (await text()));
  }
  console.log(kind, await browser.version(), '· POSTs:', posts.join(', ') || 'none seen');
} finally {
  await browser.close();
}
if (fails.length) {
  console.log('FAIL', fails.join(' | '));
  process.exit(1);
}
console.log('PASS: login, save, logout');
