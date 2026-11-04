#!/usr/bin/env node
// RaiseSteady Guard: builds shared/videos.json, the built-in list of recent videos of the curated
// channels (shared/channels.json). The extension shows it when YouTube's channel feeds do not answer
// (they answer 404 on and off) and merges it with the feeds when they do (P2-161, Y2).
//
// Reuses yt-dlp (https://github.com/yt-dlp/yt-dlp, Unlicense) to read each channel's Videos tab:
//   yt-dlp --flat-playlist -J --playlist-end 12 --extractor-args youtubetab:approximate_date \
//          https://www.youtube.com/channel/<id>/videos
// Only its JSON output is read (id, title, approximate date, live status). No API key; build time only.
// The Videos tab lists no Shorts. Titles on the topic list (shared/topics.json) are left out.
//
// Usage: YTDLP=/path/to/yt-dlp node tools/update-videos.mjs [--per 12] [--cache dir] [--offline]
//   --cache dir : keep each channel's raw answer there (re-runs read it instead of asking again)
//   --offline   : build from the cache only
// A channel that fails is reported and kept from the previous shared/videos.json when it has one.
import fs from 'node:fs';
import path from 'node:path';
import { execFile } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SHARED = path.join(ROOT, 'shared');
const OUT = path.join(SHARED, 'videos.json');
const arg = (name, def) => { const i = process.argv.indexOf(name); return i > 0 && process.argv[i + 1] ? process.argv[i + 1] : def; };
const PER = Math.max(1, Math.min(30, Number(arg('--per', 12)) || 12));
const CACHE = arg('--cache', '');
const OFFLINE = process.argv.includes('--offline');
const YTDLP = process.env.YTDLP || 'yt-dlp';
const ID_RE = /^[\w-]{11}$/;
const CH_RE = /^UC[\w-]{22}$/;
const CONCURRENCY = 3;

// The topic matcher (ext/content/topics.js is a classic script that also works under node).
const require = createRequire(import.meta.url);
require(path.join(ROOT, 'ext', 'content', 'topics.js'));
const topics = globalThis.KGTopics.create(JSON.parse(fs.readFileSync(path.join(SHARED, 'topics.json'), 'utf8')));

const channels = (JSON.parse(fs.readFileSync(path.join(SHARED, 'channels.json'), 'utf8')).channels || []).filter((c) => CH_RE.test(c.id));
let previous = {};
try { previous = JSON.parse(fs.readFileSync(OUT, 'utf8')).channels || {}; } catch { /* first run */ }
if (CACHE) fs.mkdirSync(CACHE, { recursive: true });

function runYtdlp(id) {
  return new Promise((resolve, reject) => {
    const args = ['--flat-playlist', '-J', '--playlist-end', String(PER), '--socket-timeout', '20', '--no-warnings',
      '--extractor-args', 'youtubetab:approximate_date', `https://www.youtube.com/channel/${id}/videos`];
    execFile(YTDLP, args, { timeout: 120000, maxBuffer: 32 * 1024 * 1024 }, (err, stdout, stderr) => {
      if (err) return reject(new Error(String(stderr || err.message).trim().split('\n').pop().slice(0, 200)));
      try { resolve(JSON.parse(stdout)); } catch { reject(new Error('not JSON')); }
    });
  });
}

const clean = (s, n) => String(s ?? '').replace(/[\u0000-\u001f\u007f]+/g, ' ').replace(/\s+/g, ' ').trim().slice(0, n);

/** The fields we keep from one yt-dlp playlist answer. Pure. */
export function pickVideos(data, name, per = PER) {
  const out = [];
  for (const e of (data && Array.isArray(data.entries) ? data.entries : [])) {
    if (!e || !ID_RE.test(e.id) || typeof e.title !== 'string') continue;
    if (e.live_status === 'is_live' || e.live_status === 'is_upcoming') continue;
    if (e.availability && e.availability !== 'public' && e.availability !== 'unlisted') continue; // members-only, needs sign-in
    if (/^\[(private|deleted) video\]$/i.test(e.title)) continue;
    const title = clean(e.title, 200);
    if (!title || topics.matchTitle(title).hit) continue;
    const ts = Number(e.timestamp || e.release_timestamp);
    out.push({ id: e.id, title, channel: name, published: Number.isFinite(ts) && ts > 0 ? new Date(ts * 1000).toISOString().slice(0, 10) : '' });
    if (out.length >= per) break;
  }
  return out;
}

async function one(c) {
  // The cache keeps yt-dlp's answer as it came (<id>.raw.json); an empty or broken file counts as a failure.
  const cacheFile = CACHE ? path.join(CACHE, `${c.id}.raw.json`) : '';
  let data = null;
  if (cacheFile && fs.existsSync(cacheFile)) {
    try { data = JSON.parse(fs.readFileSync(cacheFile, 'utf8')); } catch { data = null; }
    if (!data && OFFLINE) throw new Error('yt-dlp gave no video list (no public Videos tab?)');
  }
  if (!data) {
    data = await runYtdlp(c.id);
    if (cacheFile) fs.writeFileSync(cacheFile, JSON.stringify(data));
  }
  if (!data || !Array.isArray(data.entries)) throw new Error('yt-dlp gave no video list (no public Videos tab?)');
  data = { channel: data.channel || data.uploader || '', entries: data.entries };
  const vids = pickVideos(data, clean(c.name || data.channel || '', 100));
  if (!vids.length) throw new Error('no videos');
  return vids;
}

const result = {}, failed = [];
let next = 0;
async function worker() {
  while (next < channels.length) {
    const c = channels[next++];
    try { result[c.id] = await one(c); process.stdout.write('.'); }
    catch (e) {
      failed.push(`${c.id} ${c.name}: ${e.message}`);
      if (previous[c.id] && previous[c.id].length) result[c.id] = previous[c.id];
      process.stdout.write('x');
    }
  }
}
await Promise.all(Array.from({ length: CONCURRENCY }, worker));
process.stdout.write('\n');

const ordered = {};
for (const c of channels) if (result[c.id]) ordered[c.id] = result[c.id];
const total = Object.values(ordered).reduce((n, v) => n + v.length, 0);
const body = { version: 1, note: 'Built by tools/update-videos.mjs from each channel\'s public Videos tab (yt-dlp). Never edit by hand.', built: new Date().toISOString().slice(0, 10), channels: ordered };
fs.writeFileSync(OUT, JSON.stringify(body, null, 1) + '\n');
console.log(`[update-videos] ${Object.keys(ordered).length}/${channels.length} channels, ${total} videos -> shared/videos.json`);
if (failed.length) console.log(`[update-videos] failed (${failed.length}):\n  ${failed.join('\n  ')}`);
