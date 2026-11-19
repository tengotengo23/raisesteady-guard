#!/usr/bin/env node
// P2-260: builds and signs the update manifest that every installed Guard reads.
// Usage: node tools/sign-update.mjs vX.Y.Z [--dry-run] [--key path] [--repo owner/name]
// Reads the release's assets with gh, hashes the real files, signs update.json with the Ed25519 key
// (default ~/Developer/keys/guard-update/ed25519-private.pem; never in the repository) and uploads
// update.json and update.json.sig to the same release. Installed copies read
// https://github.com/<repo>/releases/latest/download/update.json
import { execFileSync } from 'node:child_process';
import { createHash, createPrivateKey, sign, createPublicKey, verify } from 'node:crypto';
import { readFileSync, writeFileSync, mkdtempSync, rmSync, readdirSync } from 'node:fs';
import { tmpdir, homedir } from 'node:os';
import { join } from 'node:path';

const args = process.argv.slice(2);
const tag = args.find((a) => /^v\d+\.\d+\.\d+$/.test(a));
const dry = args.includes('--dry-run');
const opt = (n, d) => (args.includes(n) ? args[args.indexOf(n) + 1] : d);
const repo = opt('--repo', 'tengotengo23/raisesteady-guard');
const keyPath = opt('--key', join(homedir(), 'Developer/keys/guard-update/ed25519-private.pem'));
if (!tag) { console.error('usage: sign-update.mjs vX.Y.Z [--dry-run]'); process.exit(2); }

const gh = (...a) => execFileSync('gh', a, { encoding: 'utf8', maxBuffer: 64 << 20 });
const dir = mkdtempSync(join(tmpdir(), 'guard-update-'));
const sha = (f) => createHash('sha256').update(readFileSync(f)).digest('hex');
const rel = (t) => JSON.parse(gh('release', 'view', t, '-R', repo, '--json', 'assets,body,tagName,publishedAt'));
const base = `https://github.com/${repo}/releases/download`;
const version = tag.slice(1);

try {
  const r = rel(tag);
  const names = r.assets.map((a) => a.name);
  const want = {
    'windows-amd64': 'RaiseSteadyGuardSetup.exe',
    'linux-amd64-deb': `raisesteady-guard_${version}-1_amd64.deb`,
    'linux-arm64-deb': `raisesteady-guard_${version}-1_arm64.deb`,
    'linux-amd64-rpm': `raisesteady-guard-${version}-1.x86_64.rpm`,
    'linux-arm64-rpm': `raisesteady-guard-${version}-1.aarch64.rpm`,
    'linux-amd64-tar': `raisesteady-guard-${version}-linux-amd64.tar.gz`,
    'linux-arm64-tar': `raisesteady-guard-${version}-linux-arm64.tar.gz`,
  };
  const files = {};
  for (const [k, name] of Object.entries(want)) {
    if (!names.includes(name)) throw new Error(`release ${tag} has no ${name}`);
    gh('release', 'download', tag, '-R', repo, '-p', name, '-D', dir, '--clobber');
    const f = join(dir, name);
    files[k] = { name, url: `${base}/${tag}/${name}`, sha256: sha(f), size: readFileSync(f).length };
  }

  // Android: the newest release that carries an APK (the app has its own version).
  let android = null;
  for (const row of JSON.parse(gh('release', 'list', '-R', repo, '--json', 'tagName,isPrerelease', '-L', '30'))) {
    if (row.isPrerelease) continue;
    const rr = rel(row.tagName);
    const apk = rr.assets.map((a) => a.name).find((n) => /^raisesteady-guard-android-\d+\.\d+\.\d+\.apk$/.test(n));
    if (!apk) continue;
    gh('release', 'download', row.tagName, '-R', repo, '-p', apk, '-D', dir, '--clobber');
    const f = join(dir, apk);
    const v = apk.match(/(\d+)\.(\d+)\.(\d+)\.apk$/);
    // The certificate comes from the APK itself (apksigner), never from the editable release notes.
    const sdk = process.env.ANDROID_HOME || join(homedir(), 'Library/Android/sdk');
    const bt = join(sdk, 'build-tools');
    const ver = readdirSync(bt).sort().pop();
    const out = execFileSync(join(bt, ver, 'apksigner'), ['verify', '--print-certs', f], {
      encoding: 'utf8', env: { ...process.env, JAVA_HOME: process.env.JAVA_HOME || '/opt/homebrew/opt/openjdk@17' },
    });
    const cert = (out.match(/certificate SHA-256 digest:\s*([0-9a-fA-F]{64})/) || [])[1];
    if (!cert) throw new Error(`apksigner printed no SHA-256 certificate for ${apk}`);
    android = {
      name: apk, url: `${base}/${row.tagName}/${apk}`, sha256: sha(f), size: readFileSync(f).length,
      version: `${v[1]}.${v[2]}.${v[3]}`, versionCode: +v[1] * 1000000 + +v[2] * 1000 + +v[3],
      cert_sha256: cert.toLowerCase(),
    };
    break;
  }

  const manifest = { v: 1, version, date: (r.publishedAt || '').slice(0, 10), files };
  if (android) manifest.android = android;
  const body = JSON.stringify(manifest, null, 2) + '\n';

  const priv = createPrivateKey(readFileSync(keyPath));
  const sig = sign(null, Buffer.from(body), priv).toString('base64');
  const ok = verify(null, Buffer.from(body), createPublicKey(priv), Buffer.from(sig, 'base64'));
  if (!ok) throw new Error('signature does not verify');
  writeFileSync(join(dir, 'update.json'), body);
  writeFileSync(join(dir, 'update.json.sig'), sig + '\n');
  console.log(body);
  console.log(`signature ${sig.slice(0, 16)}… verifies against the key's public half`);
  if (dry) { console.log('--dry-run: nothing uploaded'); }
  else {
    gh('release', 'upload', tag, '-R', repo, '--clobber', join(dir, 'update.json'), join(dir, 'update.json.sig'));
    console.log(`uploaded update.json and update.json.sig to ${tag}`);
  }
} finally { rmSync(dir, { recursive: true, force: true }); }
