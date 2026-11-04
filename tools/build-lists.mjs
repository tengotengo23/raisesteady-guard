#!/usr/bin/env node
// RaiseSteady Guard: builds the category rulesets in ext/rules/ from open lists,
// and shared/doh.json (encrypted DNS servers) for the Android filter.
// Node 20+, no dependencies (uses the system `tar` and `curl`-free fetch).
//
// Usage:  node tools/build-lists.mjs [--refresh]
//   default   : download only what is missing from .cache/, then build.
//   --refresh : download every source again (the sources move; then SOURCES.md needs new dates).
// The same cache gives the same output byte for byte (no timestamps, stable sort).
//
// Licences and attributions: SOURCES.md.

import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import net from 'node:net';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CACHE = path.join(ROOT, '.cache');
const RULES = path.join(ROOT, 'ext', 'rules');
const SHARED = path.join(ROOT, 'shared');
const REFRESH = process.argv.includes('--refresh');

// ---- pinned sources --------------------------------------------------------------
const BLP = 'https://raw.githubusercontent.com/blocklistproject/Lists/ee3bdbaddf49c756cf3340bb2e6a9dee5282d1c6/alt-version/';
const UT1 = 'https://dsi.ut-capitole.fr/blacklists/download/';
const HAGEZI = 'https://raw.githubusercontent.com/hagezi/dns-blocklists/eb7db8ed27237af2f3c0cb79c9c735e3e076dbad/';
const SOURCES = {
  'blp-porn':      { url: BLP + 'porn-nl.txt',      file: 'bl-porn.txt' },
  'blp-gambling':  { url: BLP + 'gambling-nl.txt',  file: 'bl-gambling.txt' },
  'ut1-adult':     { url: UT1 + 'adult.tar.gz',     file: 'ut1-adult.tgz',      tar: 'adult/domains' },
  'ut1-gambling':  { url: UT1 + 'gambling.tar.gz',  file: 'ut1-gambling.tgz',   tar: 'gambling/domains' },
  'ut1-redirector':{ url: UT1 + 'redirector.tar.gz',file: 'ut1-redirector.tgz', tar: 'redirector/domains' },
  'ut1-vpn':       { url: UT1 + 'vpn.tar.gz',       file: 'ut1-vpn.tgz',        tar: 'vpn/domains' },
  'easylist':      { url: 'https://easylist.to/easylist/easylist.txt',       file: 'easylist.txt' },
  'easyprivacy':   { url: 'https://easylist.to/easylist/easyprivacy.txt',    file: 'easyprivacy.txt' },
  'majestic':      { url: 'https://downloads.majestic.com/majestic_million.csv', file: 'majestic.csv' },
  // encrypted DNS (DoH, DoT, DoQ) servers, for the Android VPN filter (shared/doh.json)
  'hagezi-doh':    { url: HAGEZI + 'wildcard/doh-onlydomains.txt', file: 'hagezi-doh-onlydomains.txt' },
  'hagezi-doh-ips':{ url: HAGEZI + 'ips/doh.txt',     file: 'hagezi-doh-ips.txt' },
};

// ---- budgets (domains kept per category; every category domain is stored twice: redirect + block) ----
const CAP = { adult: 110000, gambling: 35000, bypass: 25000 };
const MAX_TOTAL_BYTES = 12 * 1024 * 1024;
const CHUNK = 1000;
const MAIN_ONLY = ['main_frame'];

// ---- hand lists --------------------------------------------------------------------
const BYPASS_SEEDS = `
omegle.com ome.tv chatroulette.com emeraldchat.com chatspin.com camsurf.com shagle.com chatrandom.com
tinychat.com monkey.app bazoocam.com chatiw.com coomeet.com joingy.com thundr.com chatous.com
proxysite.com croxyproxy.com kproxy.com hidemy.name whoer.net hide.me hidester.com proxyium.com
nordvpn.com expressvpn.com protonvpn.com windscribe.com tunnelbear.com hotspotshield.com surfshark.com
privateinternetaccess.com cyberghostvpn.com psiphon.ca torproject.org getlantern.org betternet.co
urban-vpn.com zenmate.com hola.org browsec.com ultrasurf.us vpnbook.com
4everproxy.com genmirror.com translate.goog web.archive.org webcache.googleusercontent.com
`.split(/\s+/).filter(Boolean);

const SOCIAL_CDNS = `
fbcdn.net fb.com fb.me messenger.com cdninstagram.com instagr.am twimg.com redd.it redditmedia.com redditstatic.com
discordapp.com discordapp.net discord.media discordcdn.com tiktokcdn.com tiktokv.com tiktokcdn-us.com
sc-cdn.net snapchat.com pinimg.com twitchcdn.net jtvnw.net ttvnw.net
`.split(/\s+/).filter(Boolean);

// Never block these (and never their parents). Learning domains are added from shared/learning-domains.json.
const ESSENTIAL = `
google.com youtube.com wikipedia.org microsoft.com apple.com gstatic.com googleapis.com ytimg.com ggpht.com
cloudflare.com raisesteady.com khanacademy.org
`.split(/\s+/).filter(Boolean);

// Reviewed false positives found in the source lists (domain, reason). Kept short on purpose.
const MANUAL_EXCLUDE = new Map(Object.entries({
  // bypass: business VPN and network tools, shared CDNs, dynamic-DNS hosts and mainstream messengers (not what a child bypasses with)
  'checkpoint.com': 'security vendor', 'tailscale.com': 'business VPN', 'zerotier.com': 'business VPN', 'mitmproxy.org': 'developer tool',
  'stunnel.org': 'developer tool', 'name.com': 'domain registrar', 'msecnd.net': 'Azure CDN', 'freetranslation.com': 'translator',
  'kicks-ass.net': 'dynamic DNS', 'noip.com': 'dynamic DNS', 'userapi.com': 'VK CDN', 'radiofarda.com': 'news site',
  'honeygain.com': 'income app', 'emergent.sh': 'not a proxy', 'imvu.com': 'game', 'discord.com': 'social list', 'discordapp.com': 'social list',
  'snapchat.com': 'social list', 'vk.com': 'social list', 'messenger.com': 'social list', 'skype.com': 'messenger', 'icq.com': 'messenger',
  'livechat.com': 'support chat tool', 'livechatinc.com': 'support chat tool', 'liveperson.com': 'support chat tool', 'crisp.chat': 'support chat tool',
  // adult and gambling: mainstream, health, sex-education, news and shop sites that the source lists tag too broadly
  ...Object.fromEntries(`na.org gamblersanonymous.org cosmopolitan.com aktion-mensch.de mundodeportivo.com fasthosts.co.uk sa.com 51.la le.com pclady.com.cn
  breakingnews.ie fashionista.com chicagoreader.com portlandmercury.com thestranger.com femalefirst.co.uk ok.co.uk queerty.com thefrisky.com jokesgallery.com
  b3ta.com rtalabel.org cannabis.net modelmayhem.com models.com yes24.com bekkoame.ne.jp videa.hu xxlmag.com hotbot.com collegehumor.com break.com
  bitdegree.org cex.io bitz.io europeangaming.eu fark.com rb.ru scarleteen.com kinseyinstitute.org gaystarnews.com pride.com queer.de gaycenter.org
  siecus.org maggies.org aquateencentral.com recipegirl.com budweiser.com covergirl.com hooters.com furnituretoday.com ambitionbox.com sfbg.com nerve.com
  bust.com afterellen.com justintimberlake.com heatworld.com starmagazine.com whosdatedwho.com celebitchy.com wonderwall.com peoplestylewatch.com alloy.com
  latina.com fhm.com mikocon.com mangaupdates.com toonopedia.com dieselsweeties.com motherfuckingwebsite.com empr.com dansdata.com cda.pl lacuarta.com
  bookmark.com taptap.io vso.org.uk nva.org.uk aukro.cz freespeechcoalition.com alliance-francaise-des-designers.org invader.be newsghana.com.gh
  asaecenter.org pieria.co.uk isorg.fr adivbois.org jra.go.jp sierraspace.com wellpoint.com isquaredsoftware.com supertokens.com seeq.com thrivehive.com
  cmle.ru grazitti.com wideopenwest.com domainsbyproxy.com dl.am stfi.re feltmagnet.com jmarshall.com 99k.org victoryrecords.com fluidstack.io dmegs.com
  rosinstrument.com ewon.biz page.ne.jp zone.ne.jp pegasusdirectory.com topshareware.com webqr.com megaleecher.net alldebrid.com annie.ne.jp net-filter.com
  youngzsoft.net mob.com cityheaven.net hot.ee smotri.com gportal.hu gifs.com weheartit.com`.split(/\s+/).filter(Boolean).map((d) => [d, 'mainstream or too broad in the source lists'])),
  // found by the false-positive sample of kid/school/baby/edu/health names (checked by page title or a dead domain with a child or school name)
  ...Object.fromEntries(`0-12kids.com 0555edu.com metrostate.edu ohiochristian.edu babynet.pl babysitters.pl care4baby.net canchild.com childcover.com
  doctorg.com seanbaby.com wikid.net theducky.com stuntkid.com collegerules.com sleduj.to reductress.com kuskov.family kids-can.org kidshands.com
  kidzilla.info playkids.nl dzedu.com edu-osa.ru family.h12.ru cfcollege.com bidoctors.com dxmedical.eu health-f.com keduku.com leduo.net museum.imp.dk
  redux.dk familygid.com medicuro.com`.split(/\s+/).filter(Boolean).map((d) => [d, 'false positive found in the review sample'])),
  'pidgin.im': 'messenger', 'threema.ch': 'messenger', 'chat.ru': 'portal', 'miranda-im.org': 'messenger', 'meebo.com': 'messenger',
}));
// Schools, universities, governments and the military are never adult sites, whatever the source list says.
const INSTITUTION = /\.(edu|gov|mil|int)$|\.(edu|gov|ac|sch|mil)\.[a-z]{2}$|\.(k12|sch)\.[a-z]{2}$|\.k12\.[a-z]{2}\.us$/;

// ---- helpers -------------------------------------------------------------------------
const sha = (buf) => crypto.createHash('sha256').update(buf).digest('hex');
const HOST = /^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$/;
function norm(s) {
  s = String(s).trim().toLowerCase().replace(/^www\./, '').replace(/\.$/, '');
  return HOST.test(s) ? s : null;
}
function suffixes(d) { // proper parent domains with at least two labels
  const p = d.split('.'); const out = [];
  for (let i = 1; i < p.length - 1; i++) out.push(p.slice(i).join('.'));
  return out;
}

async function ensure(name) {
  const s = SOURCES[name]; const f = path.join(CACHE, s.file);
  if (!REFRESH && fs.existsSync(f) && fs.statSync(f).size > 0) return f;
  const r = await fetch(s.url, { headers: { 'user-agent': 'RaiseSteadyGuard-build-lists/1 (open-source list builder)' } });
  if (!r.ok) throw new Error(`download failed ${name}: ${r.status} ${s.url}`);
  fs.writeFileSync(f, Buffer.from(await r.arrayBuffer()));
  return f;
}
function readLines(name) {
  const s = SOURCES[name]; const f = path.join(CACHE, s.file);
  const text = s.tar
    ? execFileSync('tar', ['-xzOf', f, s.tar], { maxBuffer: 1 << 30 }).toString('utf8')
    : fs.readFileSync(f, 'utf8');
  return text.split(/\r?\n/);
}
function domainList(name) { // hosts-style, plain domain files
  const out = new Set();
  for (const l of readLines(name)) {
    if (!l || l[0] === '#' || l[0] === '!') continue;
    const d = norm(l.trim().split(/\s+/).pop());
    if (d) out.add(d);
  }
  return out;
}
function easylistDomains(name) { // only pure ||domain^ rules (no options, no paths)
  const out = new Set();
  for (const l of readLines(name)) {
    const m = /^\|\|([a-z0-9.-]+)\^$/.exec(l);
    if (m) { const d = norm(m[1]); if (d) out.add(d); }
  }
  return out;
}

// ---- main -----------------------------------------------------------------------------
fs.mkdirSync(CACHE, { recursive: true });
fs.mkdirSync(RULES, { recursive: true });
const manifest = {};
for (const name of Object.keys(SOURCES)) {
  const f = await ensure(name);
  const buf = fs.readFileSync(f);
  manifest[name] = { url: SOURCES[name].url, bytes: buf.length, sha256: sha(buf), fetched: fs.statSync(f).mtime.toISOString().slice(0, 10) };
}
fs.writeFileSync(path.join(CACHE, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');

// popularity: Majestic Million rank by registrable domain
const rank = new Map();
{
  const lines = readLines('majestic');
  for (let i = 1; i < lines.length; i++) {
    const p = lines[i].split(',');
    if (p[2]) rank.set(p[2].toLowerCase(), Number(p[0]));
  }
}
const rankOf = (d) => rank.get(d) ?? Infinity; // exact registrable domain only (subdomains of big hosts such as blogspot.com are not popular)

// protected domains
const learning = JSON.parse(fs.readFileSync(path.join(SHARED, 'learning-domains.json'), 'utf8')).domains;
// entries with a path (bbc.co.uk/bitesize) protect their host from the block lists
const PROTECTED = [...new Set([...ESSENTIAL, ...learning.map((d) => d.toLowerCase().split('/')[0])])].sort();
const PROT_SET = new Set(PROTECTED);
const PROT_PARENTS = new Set(); // parents of protected entries (blocking them would block the protected one)
for (const p of PROTECTED) for (const q of suffixes(p)) PROT_PARENTS.add(q);
const isProtected = (d) => PROT_SET.has(d) || PROT_PARENTS.has(d) || suffixes(d).some((x) => PROT_SET.has(x));

function dropParented(set) { // requestDomains also matches subdomains, so a child of a listed domain is redundant
  const out = [];
  for (const d of set) if (!suffixes(d).some((s) => set.has(s))) out.push(d);
  return out;
}

function select(sourceSets, seeds, cap, label, minVotes = 1) {
  // sourceSets: Map(sourceName -> Set). Count how many sources list each domain.
  const votes = new Map();
  for (const [, set] of sourceSets) for (const d of set) votes.set(d, (votes.get(d) || 0) + 1);
  for (const d of seeds) votes.set(d, (votes.get(d) || 0) + 5);
  const stats = { candidates: votes.size, protectedDropped: 0, excluded: 0 };
  const all = new Set();
  for (const d of votes.keys()) {
    if (isProtected(d)) { stats.protectedDropped++; continue; }
    if (MANUAL_EXCLUDE.has(d) || INSTITUTION.test(d)) { stats.excluded++; continue; }
    all.add(d);
  }
  const unique = dropParented(all);
  stats.afterParentDedupe = unique.length;
  const seedSet = new Set(seeds);
  const lt = (x, y) => (x < y ? -1 : x > y ? 1 : 0);
  const tiers = [[], [], [], []]; // seeds, popular, listed by several sources, the rest
  for (const d of unique) {
    if (seedSet.has(d)) tiers[0].push(d);
    else if (rankOf(d) !== Infinity && votes.get(d) >= minVotes) tiers[1].push(d);
    else if (votes.get(d) >= 2) tiers[2].push(d);
    else tiers[3].push(d);
  }
  tiers[0].sort(lt);
  tiers[1].sort((a, b) => rankOf(a) - rankOf(b) || lt(a, b));
  const shortFirst = (a, b) => a.length - b.length || lt(a, b);
  tiers[2].sort(shortFirst);
  if (tiers[0].length + tiers[1].length + tiers[2].length < cap) tiers[3].sort(shortFirst); else tiers[3] = [];
  const ordered = tiers.flat();
  stats.tiers = { seeds: tiers[0].length, popular: tiers[1].length, severalSources: tiers[2].length };
  const kept = ordered.slice(0, cap);
  stats.keptPopular = kept.filter((d) => rankOf(d) !== Infinity).length;
  stats.kept = kept.length;
  stats.leftOut = unique.length - kept.length;
  return { domains: kept.sort(), stats };
}

function chunks(arr, n) { const out = []; for (let i = 0; i < arr.length; i += n) out.push(arr.slice(i, i + n)); return out; }

function categoryRules(reason, domains) {
  const rules = []; let id = 1;
  for (const c of chunks(domains, CHUNK)) {
    rules.push({ id: id++, priority: 1, action: { type: 'redirect', redirect: { extensionPath: `/pages/blocked.html?reason=${reason}` } }, condition: { requestDomains: c, resourceTypes: MAIN_ONLY } });
    rules.push({ id: id++, priority: 1, action: { type: 'block' }, condition: { requestDomains: c, excludedResourceTypes: MAIN_ONLY } });
  }
  return rules;
}
function adsRules(domains) {
  return chunks(domains, CHUNK).map((c, i) => ({ id: i + 1, priority: 1, action: { type: 'block' }, condition: { requestDomains: c, excludedResourceTypes: MAIN_ONLY } }));
}

const report = {};
const out = {};

// adult
{
  const r = select(new Map([['blp', domainList('blp-porn')], ['ut1', domainList('ut1-adult')]]), [], CAP.adult, 'adult', 2);
  out.adult = categoryRules('adult', r.domains); report.adult = r.stats;
}
// gambling
{
  const r = select(new Map([['blp', domainList('blp-gambling')], ['ut1', domainList('ut1-gambling')]]), [], CAP.gambling, 'gambling', 2);
  out.gambling = categoryRules('gambling', r.domains); report.gambling = r.stats;
}
// bypass: web proxies, VPNs, chat with strangers
{
  const r = select(new Map([['redirector', domainList('ut1-redirector')], ['vpn', domainList('ut1-vpn')]]),
    BYPASS_SEEDS.map(norm).filter(Boolean), CAP.bypass, 'bypass');
  out.bypass = categoryRules('bypass', r.domains); report.bypass = r.stats;
}
// social: the feed sites from shared/social-domains.json plus their asset domains
{
  const feeds = JSON.parse(fs.readFileSync(path.join(SHARED, 'social-domains.json'), 'utf8')).feeds.map((f) => norm(f.domain));
  const set = new Set([...feeds, ...SOCIAL_CDNS.map(norm)].filter(Boolean));
  const kept = dropParented(new Set([...set].filter((d) => !isProtected(d)))).sort();
  out.social = categoryRules('social', kept); report.social = { candidates: set.size, kept: kept.length };
}
// ads: pure ||domain^ rules of EasyList and EasyPrivacy, plus Block List Project ads that are also popular
{
  const el = easylistDomains('easylist'), ep = easylistDomains('easyprivacy');
  const all = new Set([...el, ...ep]);
  let dropped = 0;
  const ok = new Set([...all].filter((d) => { if (isProtected(d)) { dropped++; return false; } return true; }));
  const kept = dropParented(ok).sort();
  out.ads = adsRules(kept);
  report.ads = { easylist: el.size, easyprivacy: ep.size, protectedDropped: dropped, kept: kept.length };
}

// encrypted DNS: host names of DoH/DoT servers and DoH server addresses, for the Android filter.
// Written to shared/doh.json (not a browser ruleset). Our own upstream (AdGuard DNS Family, as
// agent core.go and iOS Model.swift) and its parents are left out of `domains`, or the suffix match
// would block it; its addresses stay in `ips` (Guard's own traffic is outside the VPN).
// Helplines (shared/topics.json) and protected domains or their parents are never listed.
const UPSTREAM_HOSTS = ['family.adguard-dns.com'];
const CANARY = 'use-application-dns.net'; // Firefox turns its own DoH off when this name does not resolve
{
  const helplines = JSON.parse(fs.readFileSync(path.join(SHARED, 'topics.json'), 'utf8')).helplines.map(norm).filter(Boolean);
  const never = new Set(UPSTREAM_HOSTS.flatMap((h) => [h, ...suffixes(h)]));
  for (const h of helplines) for (const x of [h, ...suffixes(h)]) never.add(x);
  const src = domainList('hagezi-doh');
  const kept = new Set([CANARY]);
  const dropped = [];
  for (const d of src) {
    if (never.has(d) || PROT_SET.has(d) || PROT_PARENTS.has(d) || suffixes(d).some((x) => helplines.includes(x))) { dropped.push(d); continue; }
    kept.add(d);
  }
  const ips = new Set();
  for (const l of readLines('hagezi-doh-ips')) {
    const t = l.trim();
    if (t && t[0] !== '#' && net.isIP(t)) ips.add(t);
  }
  const lt = (x, y) => (x < y ? -1 : x > y ? 1 : 0);
  const doh = {
    version: 1,
    note: 'Encrypted DNS servers, for the Android filter: names get no answer, DoH addresses are routed into the VPN and only DNS on port 53 passes. Built by tools/build-lists.mjs from HaGeZi DNS blocklists (GPL-3.0), see SOURCES.md. Do not edit by hand.',
    source: { url: HAGEZI, files: ['wildcard/doh-onlydomains.txt', 'ips/doh.txt'], sha256: { domains: manifest['hagezi-doh'].sha256, ips: manifest['hagezi-doh-ips'].sha256 } },
    domains: [...kept].sort(lt),
    ips: [...ips].sort(lt),
  };
  fs.writeFileSync(path.join(SHARED, 'doh.json'), JSON.stringify(doh, null, 1) + '\n');
  report.doh = { source: src.size, kept: doh.domains.length, dropped: dropped.sort(lt), ips: doh.ips.length };
}

// ---- write + validate -----------------------------------------------------------------
let total = 0;
const problems = [];
for (const [name, rules] of Object.entries(out)) {
  const file = path.join(RULES, name + '.json');
  const text = JSON.stringify(rules);
  fs.writeFileSync(file, text + '\n');
  total += Buffer.byteLength(text) + 1;
  // validate by reading it back
  const back = JSON.parse(fs.readFileSync(file, 'utf8'));
  const ids = new Set(); let domainsInFile = 0;
  for (const r of back) {
    if (!Number.isInteger(r.id) || ids.has(r.id)) problems.push(`${name}: bad or duplicate id ${r.id}`);
    ids.add(r.id);
    if (!r.action || !r.action.type || !r.condition) problems.push(`${name}: rule ${r.id} lacks action or condition`);
    for (const d of r.condition.requestDomains || []) {
      domainsInFile++;
      if (isProtected(d)) problems.push(`${name}: protected domain slipped in: ${d}`);
      if (!HOST.test(d)) problems.push(`${name}: bad domain ${d}`);
    }
  }
  report[name].fileBytes = Buffer.byteLength(text) + 1;
  report[name].rules = back.length;
  report[name].domainEntries = domainsInFile;
}
report._total = { bytes: total, mb: +(total / 1048576).toFixed(2), protectedCount: PROTECTED.length };
fs.writeFileSync(path.join(CACHE, 'build-report.json'), JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify(report, null, 1));
if (total > MAX_TOTAL_BYTES) problems.push(`rulesets total ${total} bytes, over the ${MAX_TOTAL_BYTES} limit`);
if (problems.length) { console.error('BUILD FAILED:\n' + problems.slice(0, 30).join('\n')); process.exit(1); }
console.log('OK: all rulesets valid, total', (total / 1048576).toFixed(2), 'MB');
