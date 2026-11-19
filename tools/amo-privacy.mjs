#!/usr/bin/env node
// RaiseSteady Guard (P2-260): read or replace the privacy policy on the Firefox add-on listing
// (addons.mozilla.org API v5, endpoint addons/addon/<guid>/eula_policy/). Same JWT client as amo-sign.mjs.
// Usage:
//   node tools/amo-privacy.mjs get <out.json>                 save the live policy, every language (a backup)
//   node tools/amo-privacy.mjs set <text file> [--lang en-US] [--dry-run]
// The key file (--key, default ~/.config/nexodyne/amo.json) holds { issuer, secret }; neither is ever printed.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';

const API = 'https://addons.mozilla.org/api/v5';
const ADDON = 'guard@raisesteady.com';
const [cmd, file] = process.argv.slice(2).filter((a) => !a.startsWith('--'));
const arg = (n, d) => { const i = process.argv.indexOf(n); return i > 0 && process.argv[i + 1] ? process.argv[i + 1] : d; };
const keyFile = arg('--key', path.join(os.homedir(), '.config', 'nexodyne', 'amo.json'));
const lang = arg('--lang', 'en-US');
const dry = process.argv.includes('--dry-run');
if (!['get', 'set'].includes(cmd) || !file) { console.error('Usage: node tools/amo-privacy.mjs get <out.json> | set <text file> [--lang en-US] [--dry-run]'); process.exit(2); }
const { issuer, secret } = JSON.parse(fs.readFileSync(keyFile, 'utf8'));
if (!issuer || !secret) { console.error('key file lacks issuer or secret'); process.exit(2); }

const b64u = (b) => Buffer.from(b).toString('base64').replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_');
function jwt() {
  const now = Math.floor(Date.now() / 1000);
  const head = b64u(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const body = b64u(JSON.stringify({ iss: issuer, jti: crypto.randomUUID(), iat: now, exp: now + 240 }));
  const sig = b64u(crypto.createHmac('sha256', secret).update(`${head}.${body}`).digest());
  return `JWT ${head}.${body}.${sig}`;
}
const url = `${API}/addons/addon/${encodeURIComponent(ADDON)}/eula_policy/`;
async function call(method, body) {
  const headers = { Authorization: jwt() };
  if (body) headers['Content-Type'] = 'application/json';
  const r = await fetch(url, { method, body: body ? JSON.stringify(body) : undefined, headers });
  const text = await r.text();
  if (!r.ok) throw new Error(`${method} eula_policy: HTTP ${r.status} ${text.slice(0, 300)}`);
  return JSON.parse(text);
}
// Translated fields come back as { lang: text } (or a plain string in older shapes).
const langs = (v) => (v && typeof v === 'object' ? Object.keys(v) : v ? ['(plain)'] : []);
// Mozilla returns links wrapped in <a> tags, so compare without tags.
const textOf = (v) => ((v && typeof v === 'object' ? v[lang] : v) || '').replace(/<[^>]+>/g, '');

const live = await call('GET');
if (cmd === 'get') {
  fs.writeFileSync(file, JSON.stringify(live, null, 2) + '\n');
  console.log(`saved ${file}: privacy_policy languages ${JSON.stringify(langs(live.privacy_policy))}, ${lang} ${textOf(live.privacy_policy).length} characters`);
  process.exit(0);
}
const next = fs.readFileSync(file, 'utf8').trim();
if (!next) { console.error('empty text file'); process.exit(2); }
const others = langs(live.privacy_policy).filter((l) => l !== lang && l !== '(plain)');
console.log(`live ${lang}: ${textOf(live.privacy_policy).length} characters; new: ${next.length}; other languages on the listing: ${JSON.stringify(others)}`);
if (textOf(live.privacy_policy).trim() === next) { console.log('already current'); process.exit(0); }
if (dry) { console.log('dry run: nothing sent'); process.exit(0); }
const done = await call('PATCH', { privacy_policy: { [lang]: next } });
const ok = textOf(done.privacy_policy).trim() === next;
console.log(`sent; listing now ${ok ? 'matches' : 'DIFFERS from'} the file`);
process.exit(ok ? 0 : 1);
