// Reads the policy page of a browser already started with a debugging port,
// and prints its text (shadow roots included). Used by the Linux test run.
//   node read-policy-page.mjs cdp 9334 chrome://policy      (Chrome, Chromium, Edge, Brave)
//   node read-policy-page.mjs bidi 9335 about:policies      (Firefox, WebDriver BiDi)
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
    ws.send(JSON.stringify({ id: n, method, params }));
  });
}

async function open(wsUrl) {
  const ws = new WebSocket(wsUrl);
  await new Promise((res, rej) => { ws.addEventListener('open', res); ws.addEventListener('error', rej); });
  return ws;
}

async function cdp() {
  let target = null;
  for (let i = 0; i < 40 && !target; i++) {
    await sleep(1000);
    try {
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      target = list.find((t) => t.type === 'page');
    } catch { /* not up yet */ }
  }
  if (!target) { console.error(`no page on port ${port}`); process.exit(2); }
  const ws = await open(target.webSocketDebuggerUrl);
  const call = rpc(ws);
  // Chrome ignores chrome:// addresses given on its command line: open it from here.
  await call('Page.navigate', { url });
  await sleep(6000);
  const r = await call('Runtime.evaluate', { expression: WALK, returnByValue: true });
  ws.close();
  return r.result.value;
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
  const text = mode === 'bidi' ? await bidi() : await cdp();
  if (!text) { console.error('the page has no text'); process.exit(1); }
  process.stdout.write(text + '\n');
} catch (e) {
  console.error(String(e && e.message || e));
  process.exit(2);
}
