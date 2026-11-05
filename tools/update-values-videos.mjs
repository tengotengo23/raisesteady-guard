#!/usr/bin/env node
// RaiseSteady Guard (P2-175): the values videos, a large pool of classic fables, parables and stories on the
// themes of the RaiseSteady book (one theme per chapter, shared/values-queries.json). The extension mixes a few
// of them into the recommended grid each day (background/videos.js, rotation by day).
//
// Two steps, so nothing reaches children without a review:
//   node tools/update-values-videos.mjs --candidates out.json
//       searches YouTube's public result pages (tools/yt-public.mjs), applies the fixed filters below and writes
//       the candidates for review (the pool is not touched);
//   node tools/update-values-videos.mjs --build out.json
//       writes shared/values-videos.json from the candidates that shared/values-review.json approves.
// Filters: length 1-60 min, enough views for the language, no title or description line on the topic list,
// no clickbait or junk words, no religious channels or titles (the values are the backbone, not a label),
// none of the channels dropped in P2-175, at most 3 per channel per theme and 10 per channel and language.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { search } from './yt-public.mjs';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SHARED = path.join(ROOT, 'shared');
const arg = (name) => { const i = process.argv.indexOf(name); return i > 0 && process.argv[i + 1] ? process.argv[i + 1] : ''; };
const ID_RE = /^[\w-]{11}$/;

const require = createRequire(import.meta.url);
require(path.join(ROOT, 'ext', 'content', 'topics.js'));
const topics = globalThis.KGTopics.create(JSON.parse(fs.readFileSync(path.join(SHARED, 'topics.json'), 'utf8')), { feed: true });

export const MIN_VIEWS = { en: 20000, ru: 10000, ka: 1000 };
const TEEN_MIN_VIEWS = 50000;
export const JUNK = /(?<![\p{L}\p{N}])(?:prank\p{L}*|challenge|vs\.?|versus|tiktok|roblox|minecraft|fortnite|gacha|among us|squid game|elsa|frozen|spider-?man|superhero|joker|huggy|asmr|mukbang|surprise eggs?|unboxing|toys? review|reacts?|reaction|try not to|gone wrong|24 hours|kiss\p{L}*|love story|crush|boyfriend|girlfriend|dating|bikini|sexy|twerk|scary|ghost|shorts?|baby shark|cocomelon|peppa|blippi|skibidi|brainrot|сериал|пранк|челлендж|приколы|тикток|майнкрафт)(?![\p{L}])/iu;
export const RELIGIOUS = /(?<![\p{L}])(?:orthodox\p{L}*|православ\p{L}*|მართლმადიდებ\p{L}*|church\p{L}*|церк\p{L}*|ეკლესი\p{L}*|bible|библи\p{L}*|ბიბლი\p{L}*|jesus|иисус\p{L}*|იესო\p{L}*|christ\p{L}*|христ\p{L}*|ქრისტ\p{L}*|saints?|свят\p{L}*|წმიდა|god|бог\p{L}*|ღმერთ\p{L}*|prayer\p{L}*|молитв\p{L}*|ლოცვ\p{L}*|sunday school|veggietales|superbook|psalm\p{L}*|gospel|angels?|ангел\p{L}*|ანგელოზ\p{L}*)(?![\p{L}])/iu;
export const DROPPED = /sesame street|pbs kids|cbeebies|ms rachel|gonoodle|hey duggee|cosmic kids|bitesize|^ted$|peekaboo kidz|scratch garden|pocoyo|peppa|cocomelon|blippi/i;

function capsHeavy(title) {
  const letters = title.replace(/[^\p{L}]/gu, '');
  if (letters.length < 16) return false;
  const upper = letters.replace(/[^\p{Lu}]/gu, '').length;
  return upper / letters.length > 0.6;
}

/** Why a search result is left out ('' = keep). Pure; unit-tested. */
export function dropReason(v, lang, age) {
  if (!v || !ID_RE.test(v.id) || !v.title) return 'bad';
  if (!v.channelId) return 'no channel';
  if (!(v.lengthSec >= 60 && v.lengthSec <= 3600)) return 'length';
  if ((v.views || 0) < (age === 'teen' && lang === 'en' ? TEEN_MIN_VIEWS : MIN_VIEWS[lang] || 20000)) return 'views';
  const text = `${v.title} ${v.channel || ''}`;
  if (topics.matchTitle(text).hit || (v.desc && topics.matchTitle(v.desc).hit)) return 'topic';
  if (JUNK.test(text)) return 'junk';
  if (RELIGIOUS.test(text)) return 'religious';
  if (DROPPED.test(v.channel || '')) return 'dropped channel';
  if (capsHeavy(v.title)) return 'caps';
  return '';
}

/** Merge the results of all queries: one entry per video, themes and bands joined, channel caps applied. Pure. */
export function collect(rows, ages) {
  const byId = new Map(), perChTheme = new Map(), perChLang = new Map(), drops = {};
  for (const { v, lang, age, theme, q } of rows) {
    const why = dropReason(v, lang, age);
    if (why) { drops[why] = (drops[why] || 0) + 1; continue; }
    const bands = ages[age] || [];
    const had = byId.get(v.id);
    if (had) {
      for (const b of bands) if (!had.bands.includes(b)) had.bands.push(b);
      if (!had.themes.includes(theme)) had.themes.push(theme);
      continue;
    }
    const kt = `${v.channelId}|${theme}`, kl = `${v.channelId}|${lang}`;
    if ((perChTheme.get(kt) || 0) >= 3 || (perChLang.get(kl) || 0) >= 10) { drops.cap = (drops.cap || 0) + 1; continue; }
    perChTheme.set(kt, (perChTheme.get(kt) || 0) + 1);
    perChLang.set(kl, (perChLang.get(kl) || 0) + 1);
    byId.set(v.id, { id: v.id, title: v.title, channel: v.channel, channelId: v.channelId, lang, bands: [...bands], themes: [theme],
      lengthMin: Math.round(v.lengthSec / 60), views: v.views, desc: v.desc || '', q });
  }
  return { candidates: [...byId.values()], drops };
}

/** The pool from reviewed candidates. Pure. */
export function buildPool(candidates, review) {
  const ok = new Set(review.approve || []), no = new Set(review.reject || []), noCh = new Set(review.rejectChannels || []);
  return candidates.filter((c) => ok.has(c.id) && !no.has(c.id) && !noCh.has(c.channelId) && !topics.matchTitle(`${c.title} ${c.channel}`).hit)
    .map((c) => ({ id: c.id, title: c.title, channel: c.channel, channelId: c.channelId, lang: c.lang, bands: [...c.bands].sort(), themes: c.themes }));
}

async function main() {
  const qf = JSON.parse(fs.readFileSync(path.join(SHARED, 'values-queries.json'), 'utf8'));
  const candOut = arg('--candidates'), buildFrom = arg('--build');
  if (candOut) {
    // YouTube slows down quick runs (HTTP 429): one search every 2.5 s, up to 3 tries with longer waits;
    // --cache dir keeps each answer, so a re-run asks only for what it does not have.
    const cache = arg('--cache');
    if (cache) fs.mkdirSync(cache, { recursive: true });
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const rows = [], errors = [];
    let n = 0, failed = 0;
    for (const t of qf.themes) {
      for (const [lang, age, q] of t.queries) {
        n++;
        const file = cache ? path.join(cache, `${lang}-${q.replace(/[^\p{L}\p{N}]+/gu, '_').slice(0, 80)}.json`) : '';
        let res = null;
        if (file && fs.existsSync(file)) { try { res = JSON.parse(fs.readFileSync(file, 'utf8')); } catch { res = null; } }
        for (let i = 0; !res && i < 3; i++) {
          try { res = await search(q, { hl: lang }); if (file) fs.writeFileSync(file, JSON.stringify(res)); }
          catch (e) { if (i === 2) errors.push(`${q}: ${e.message}`); await sleep([10000, 30000, 0][i]); }
          await sleep(2500);
        }
        if (!res) { failed++; process.stdout.write('x'); continue; }
        for (const v of res) rows.push({ v, lang, age, theme: t.id, q });
        process.stdout.write('.');
      }
    }
    if (errors.length) console.log(`\n[values] failed: ${errors.slice(0, 5).join(' | ')}`);
    process.stdout.write('\n');
    const { candidates, drops } = collect(rows, qf.ages);
    fs.writeFileSync(candOut, JSON.stringify({ built: new Date().toISOString().slice(0, 10), candidates }, null, 1) + '\n');
    console.log(`[values] ${n} searches (${failed} failed), ${rows.length} results, ${candidates.length} candidates -> ${candOut}`);
    console.log(`[values] left out: ${JSON.stringify(drops)}`);
    return;
  }
  if (buildFrom) {
    const { candidates } = JSON.parse(fs.readFileSync(buildFrom, 'utf8'));
    const review = JSON.parse(fs.readFileSync(path.join(SHARED, 'values-review.json'), 'utf8'));
    const videos = buildPool(candidates, review);
    const body = { version: 1, note: 'Built by tools/update-values-videos.mjs from reviewed search results (shared/values-queries.json, shared/values-review.json). Never edit by hand.', built: new Date().toISOString().slice(0, 10), videos };
    fs.writeFileSync(path.join(SHARED, 'values-videos.json'), JSON.stringify(body, null, 1) + '\n');
    const by = {};
    for (const v of videos) for (const b of v.bands) by[`${v.lang} ${b}`] = (by[`${v.lang} ${b}`] || 0) + 1;
    console.log(`[values] ${videos.length} videos -> shared/values-videos.json; per language and age: ${JSON.stringify(by)}`);
    return;
  }
  console.log('Usage: --candidates out.json | --build out.json');
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) await main();
