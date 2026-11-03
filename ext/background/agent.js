// Client for the local RaiseSteady Guard agent (http://127.0.0.1:<agentPort>, header X-KG-Token).
// Never throws and never blocks protection: when the agent is down, calls return null,
// events wait in an offline queue (storage.local) and are sent on the next good call.

const TIMEOUT_MS = 4000;
const QUEUE_KEY = 'kg.agentQueue';
const QUEUE_MAX = 200;

export function browserName() {
  const ua = (globalThis.navigator && navigator.userAgent) || '';
  const brands = (globalThis.navigator && navigator.userAgentData && navigator.userAgentData.brands || []).map((b) => b.brand).join(' ');
  if (/Firefox\//.test(ua)) return 'firefox';
  if (/Edg\//.test(ua) || /Microsoft Edge/.test(brands)) return 'edge';
  if (globalThis.navigator && navigator.brave) return 'brave';
  if (/Vivaldi/.test(ua) || /Vivaldi/.test(brands)) return 'vivaldi';
  return 'chrome';
}

export function createAgent({ api, getConfig, fetchImpl = (...a) => fetch(...a) }) {
  let online = null; // null = unknown

  async function call(method, path, body) {
    const cfg = getConfig();
    if (!cfg || cfg.standalone || !cfg.agentToken || !cfg.agentPort) return null;
    const ctl = new AbortController();
    const timer = setTimeout(() => ctl.abort(), TIMEOUT_MS);
    try {
      const res = await fetchImpl(`http://127.0.0.1:${cfg.agentPort}${path}`, {
        method,
        headers: { 'X-KG-Token': cfg.agentToken, ...(body ? { 'Content-Type': 'application/json' } : {}) },
        body: body ? JSON.stringify(body) : undefined,
        signal: ctl.signal, credentials: 'omit', cache: 'no-store', redirect: 'error',
      });
      online = true;
      let data = null;
      try { data = await res.json(); } catch { data = null; }
      return { status: res.status, ok: res.ok, data };
    } catch {
      online = false;
      return null;
    } finally { clearTimeout(timer); }
  }

  async function readQueue() { try { return (await api.storage.local.get(QUEUE_KEY))[QUEUE_KEY] || []; } catch { return []; } }
  async function writeQueue(q) { try { await api.storage.local.set({ [QUEUE_KEY]: q.slice(-QUEUE_MAX) }); } catch { /* storage full: drop */ } }

  async function flush() {
    const q = await readQueue();
    if (!q.length) return;
    const left = [];
    for (let i = 0; i < q.length; i++) {
      const r = await call('POST', '/v1/event', q[i]);
      if (!r) { left.push(...q.slice(i)); break; }
    }
    await writeQueue(left);
  }

  return {
    isOnline: () => online,
    async state() { const r = await call('GET', '/v1/state'); return r && r.ok ? r.data : null; },
    /** Reports usage; returns the agent state, or null when offline. */
    async usage(report) {
      const r = await call('POST', '/v1/usage', { browser: browserName(), ...report });
      if (r && r.ok) { flush().catch(() => {}); return r.data; }
      return null;
    },
    /** {ok:true} | {ok:false, locked?} | {ok:false, offline:true} */
    async pin(pin) {
      const r = await call('POST', '/v1/pin', { pin });
      if (!r) return { ok: false, offline: true };
      return { ok: !!(r.ok && r.data && r.data.ok), locked: r.status === 429 || !!(r.data && r.data.locked) };
    },
    async grant(body) {
      const r = await call('POST', '/v1/grant', body);
      if (!r) return { ok: false, offline: true };
      if (!r.ok) return { ok: false, locked: r.status === 429 || !!(r.data && r.data.locked) };
      return { ok: true, state: r.data };
    },
    async event(ev) {
      const r = await call('POST', '/v1/event', ev);
      if (!r) {
        const cfg = getConfig();
        if (cfg && !cfg.standalone) { const q = await readQueue(); q.push({ ...ev, at: new Date().toISOString() }); await writeQueue(q); }
        return false;
      }
      return true;
    },
    flush,
  };
}
