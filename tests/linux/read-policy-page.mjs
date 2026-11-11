// Reads the policy page of a browser already started with a debugging port,
// and prints its text (shadow roots included). Used by the Linux test run.
//   node read-policy-page.mjs cdp 9334 chrome://policy      (Chrome, Chromium, Edge, Brave)
//   node read-policy-page.mjs bidi 9335 about:policies      (Firefox, WebDriver BiDi)
//   node read-policy-page.mjs pages 9334                    (what the browser has open, for a failure report)
// Node 22+ (global WebSocket). Exit 2: the browser could not be reached.

const [mode, port, url] = process.argv.slice(2);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const WALK = '(function(){var o=[];function w(n){if(n.shadowRoot)w(n.shadowRoot);for(var c of n.childNodes){if(c.nodeType===3)o.push(c.textContent);else w(c);}}w(document);return o.join(" ").replace(/\\s+/g," ");})()';

function rpc(ws) {
  let id = 0;
  const waiting = new Map();
  ws.addEventListener('message', (ev) => {
    const m = JSON.parse(typeof ev.data === 'string' ? ev.data : Buffer.from(ev.data).toString());
    if (m.id && waiting.has(m.id)) {
      waiting.get(m.id)(m);
      waiting.delete(m.id);
    }
  });
  return (method, params = {}) => new Promise((resolve, reject) => {
    const n = ++id;
    waiting.set(n, (m) => (m.error ? reject(new Error(`${method}: ${JSON.stringify(m.error)} ${m.message || ''}`)) : resolve(m.result)));
    setTimeout(() => reject(new Error(`${method}: no answer in 30 s`)), 30000).unref();
    ws.send(JSON.stringify({ id: n, method, params }));
  });
}

async function open(wsUrl) {
  const ws = new WebSocket(wsUrl);
  await new Promise((res, rej) => {
    ws.addEventListener('open', res);
    ws.addEventListener('error', (e) => rej(new Error(`websocket refused: ${(e && e.message) || 'no reason given'}`)));
    setTimeout(() => rej(new Error('websocket: no answer in 15 s')), 15000).unref();
  });
  return ws;
}

async function cdp() {
  // A tab of our own. The browser's first page is not ours to use: once the
  // extension is installed, the list may start with one of its pages.
  const base = `http://127.0.0.1:${port}`;
  let tab = null;
  let last = '';
  for (let i = 0; i < 40 && !tab; i++) {
    await sleep(1000);
    try {
      const r = await fetch(`${base}/json/new?about:blank`, { method: 'PUT' });
      if (r.ok) tab = await r.json(); else last = `/json/new answers ${r.status}`;
    } catch (e) { last = String(e && e.message || e); /* not up yet */ }
  }
  if (!tab || !tab.webSocketDebuggerUrl) { console.error(`no new tab on port ${port}: ${last}`); process.exit(2); }
  try {
    const ws = await open(tab.webSocketDebuggerUrl);
    const call = rpc(ws);
    // Chrome ignores chrome:// addresses given on its command line: open it from here.
    const nav = await call('Page.navigate', { url });
    if (nav && nav.errorText) throw new Error(`${url}: ${nav.errorText}`);
    // Edge's policy page can stay blank for a while once an extension is installed: read again.
    let text = '';
    for (let i = 0; i < 4 && text.trim().length < 40; i++) {
      await sleep(6000);
      if (i === 2) await call('Page.reload', {});
      text = (await call('Runtime.evaluate', { expression: WALK, returnByValue: true })).result.value || '';
    }
    ws.close();
    return text.trim() ? text : '';
  } finally {
    try { await fetch(`${base}/json/close/${tab.id}`); } catch { /* the browser is gone */ }
  }
}

// The pages and workers the browser lists, for a failure report.
async function pages() {
  const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  return list.map((t) => `${t.type} | ${String(t.url).slice(0, 120)}`).join('\n');
}

async function bidi() {
  let ws = null;
  for (let i = 0; i < 40 && !ws; i++) {
    await sleep(1000);
    try { ws = await open(`ws://127.0.0.1:${port}/session`); } catch { ws = null; }
  }
  if (!ws) { console.error(`no BiDi on port ${port}`); process.exit(2); }
  const call = rpc(ws);
  await call('session.new', { capabilities: {} });
  const tree = await call('browsingContext.getTree', {});
  const context = tree.contexts[0].context;
  await call('browsingContext.navigate', { context, url, wait: 'complete' });
  await sleep(3000);
  const r = await call('script.evaluate', { expression: WALK, target: { context }, awaitPromise: false });
  ws.close();
  return r.result.value;
}

try {
  const text = mode === 'pages' ? await pages() : mode === 'bidi' ? await bidi() : await cdp();
  if (!text) { console.error('the page has no text'); process.exit(1); }
  process.stdout.write(text + '\n');
} catch (e) {
  console.error(String(e && e.message || e));
  process.exit(2);
}
