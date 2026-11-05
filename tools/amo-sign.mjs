#!/usr/bin/env node
// RaiseSteady Guard (P2-175): sign the Firefox build with Mozilla (addons.mozilla.org API v5), unlisted, without
// web-ext (installing it was refused by the permission check on 5 Nov). Our own short client of Mozilla's documented
// API: JWT (HS256) from the maintainer's key, upload, validation, new version, wait for the signed file, download.
// Usage: node tools/amo-sign.mjs <zip of dist/firefox> <out.xpi> [--key ~/.config/nexodyne/amo.json] [--channel unlisted]
// The key file holds { issuer, secret }; neither is ever printed.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';

const API = 'https://addons.mozilla.org/api/v5';
const ADDON = 'guard@raisesteady.com';
const [zip, out] = process.argv.slice(2).filter((a) => !a.startsWith('--'));
const arg = (n, d) => { const i = process.argv.indexOf(n); return i > 0 && process.argv[i + 1] ? process.argv[i + 1] : d; };
const keyFile = arg('--key', path.join(os.homedir(), '.config', 'nexodyne', 'amo.json'));
const channel = arg('--channel', 'unlisted');
if (!zip || !out) { console.error('Usage: node tools/amo-sign.mjs <zip> <out.xpi>'); process.exit(2); }
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
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function call(method, url, body, headers = {}) {
  const r = await fetch(url.startsWith('http') ? url : `${API}${url}`, { method, body, headers: { Authorization: jwt(), ...headers } });
  const text = await r.text();
  let json = null; try { json = JSON.parse(text); } catch { /* not JSON */ }
  if (!r.ok) throw new Error(`${method} ${url.replace(API, '')}: HTTP ${r.status} ${text.slice(0, 300)}`);
  return json;
}

// 1. upload
const form = new FormData();
form.append('upload', new Blob([fs.readFileSync(zip)], { type: 'application/zip' }), path.basename(zip));
form.append('channel', channel);
let up = await call('POST', '/addons/upload/', form);
console.log(`[amo] uploaded ${path.basename(zip)} (${channel}), waiting for validation`);
for (let i = 0; i < 60 && !up.processed; i++) { await sleep(5000); up = await call('GET', `/addons/upload/${up.uuid}/`); }
const v = up.validation || {};
console.log(`[amo] validation: valid ${up.valid}, errors ${v.errors ?? '?'}, warnings ${v.warnings ?? '?'}, notices ${v.notices ?? '?'}`);
for (const m of (v.messages || []).filter((x) => x.type === 'error' || x.type === 'warning').slice(0, 10)) console.log(`  ${m.type}: ${String(m.message).slice(0, 160)} ${m.file || ''}`);
if (!up.valid) process.exit(1);

// 2. new version from the upload
let ver = await call('POST', `/addons/addon/${encodeURIComponent(ADDON)}/versions/`, JSON.stringify({ upload: up.uuid }), { 'Content-Type': 'application/json' });
console.log(`[amo] version ${ver.version} created (id ${ver.id}), waiting for the signed file`);

// 3. wait for signing, then download
for (let i = 0; i < 120 && !(ver.file && ver.file.status === 'public' && ver.file.url); i++) {
  await sleep(10000);
  ver = await call('GET', `/addons/addon/${encodeURIComponent(ADDON)}/versions/${ver.id}/`);
}
if (!(ver.file && ver.file.url)) { console.error(`[amo] not signed yet (file status ${ver.file && ver.file.status}); run again later with the version id ${ver.id}`); process.exit(1); }
const r = await fetch(ver.file.url, { headers: { Authorization: jwt() } });
if (!r.ok) { console.error(`[amo] download HTTP ${r.status}`); process.exit(1); }
const buf = Buffer.from(await r.arrayBuffer());
fs.writeFileSync(out, buf);
console.log(`[amo] signed ${ver.version}: ${out} (${buf.length} bytes, sha256 ${crypto.createHash('sha256').update(buf).digest('hex').slice(0, 16)}...)`);
