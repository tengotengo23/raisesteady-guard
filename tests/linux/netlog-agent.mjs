// Reads a browser's network log (--log-net-log=<file>) and says what the agent
// answered to the extension. Used by the Linux test run (P2-194).
//   node netlog-agent.mjs <netlog.json> [port]
// Exit 0 when a POST to /v1/usage got 200: the agent takes that report only
// with the right token, from our extension's own address. So a 200 shows the
// extension is switched on, has its managed settings and reaches the agent.
// Only the method, the path and the answer's number are printed: the log also
// holds the request's token, which must never reach a test log.
import { createReadStream } from 'node:fs';
import { createInterface } from 'node:readline';

const [file, port = '47615'] = process.argv.slice(2);
const urlRe = new RegExp(`"url":"http://(?:127\\.0\\.0\\.1|localhost):${port}(/v1/[a-z]+)`);
const reqs = new Map(); // source id -> { path, method, status, error }

try {
  const lines = createInterface({ input: createReadStream(file), crlfDelay: Infinity });
  for await (const line of lines) {
    const src = /"source":\{"id":(\d+)/.exec(line);
    if (!src) continue;
    const id = src[1];
    const u = urlRe.exec(line);
    if (u && !reqs.has(id)) reqs.set(id, { path: u[1], method: '', status: 0, error: 0 });
    const r = reqs.get(id);
    if (!r) continue;
    const m = /"method":"([A-Z]+)"/.exec(line);
    if (m && !r.method) r.method = m[1];
    const s = /"headers":\["HTTP\/[0-9.]+ (\d{3})/.exec(line);
    if (s) r.status = Number(s[1]);
    const e = /"net_error":(-\d+)/.exec(line);
    if (e && !r.status) r.error = Number(e[1]);
  }
} catch (e) {
  console.error(`cannot read the network log: ${String(e && e.message || e)}`);
  process.exit(2);
}

const seen = new Map(); // one line per kind of request, with a count
for (const r of reqs.values()) {
  if (!r.method && !r.status && !r.error) continue; // the log names the address in other records too
  const what = `${r.method || '?'} ${r.path}: ${r.status ? `answered ${r.status}` : r.error ? `failed (network error ${r.error})` : 'no answer yet'}`;
  seen.set(what, (seen.get(what) || 0) + 1);
}
for (const [what, n] of seen) console.log(n > 1 ? `${what} (${n} times)` : what);
if (!reqs.size) console.log(`no request to the agent on port ${port} in the network log`);
const ok = [...reqs.values()].some((r) => r.path === '/v1/usage' && r.method === 'POST' && r.status === 200);
process.exit(ok ? 0 : 1);
