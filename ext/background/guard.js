// Dynamic network rules: bedtime, time over, video over, parent allow and block lists, and pages
// the page check flagged today. buildRules() and reasonFor() are pure (unit-tested); applyRules()
// and sweepTabs() talk to the browser.
//
// Priorities (static rulesets use priority 1):
//   bedtime 300 > time 200 > parent block 160 > video 150 > yt header 120 > safe search 110 >
//   parent allow 100 > page check (today: pages, labelled hosts) 90 > category lists 1
//   Learning pages under a path (bbc.co.uk/bitesize) get an allow rule at 210 while time is up.

import { inDomains, isLocalHost, isLearningUrl, isMixedHost, urlKey } from './domains.js';

export const VIDEO_DOMAINS = [
  'youtube.com', 'youtu.be', 'youtube-nocookie.com', 'vimeo.com', 'dailymotion.com', 'twitch.tv',
  'tiktok.com', 'rumble.com', 'bilibili.com', 'kick.com', 'rutube.ru', 'vkvideo.ru', 'myvideo.ge',
];
const LOCAL = ['127.0.0.1', 'localhost'];
const ALL_TYPES = ['main_frame', 'sub_frame', 'stylesheet', 'script', 'image', 'font', 'object', 'xmlhttprequest', 'ping', 'media', 'websocket', 'other'];
const NOT_MAIN = ALL_TYPES.filter((t) => t !== 'main_frame');
export const RULE_ID = { bedtime: 1, time: 2, videoMain: 3, videoFrame: 4, allow: 10, blockMain: 11, blockSub: 12, content: 20, learnPath: 30, contentUrl: 1000 };
const MAX_URL_RULES = 500;
// urlFilter treats * ^ | as special; such rare URLs are left to the page check, which runs again on load.
const plainUrl = (k) => !!k && !/[*^|]/.test(k);

const page = (reason) => ({ type: 'redirect', redirect: { extensionPath: `/pages/blocked.html?reason=${reason}` } });

/**
 * @param s { bedtime, timeOver, videoOver, allow[], block[], content[] (hosts), contentUrls[] (urlKey), learning[] (hosts), learningPaths[] ({host, path}) }
 */
export function buildRules(s) {
  const rules = [];
  const learning = s.learning || [];
  if (s.bedtime) {
    rules.push({ id: RULE_ID.bedtime, priority: 300, action: page('bedtime'), condition: { resourceTypes: ['main_frame'], excludedRequestDomains: LOCAL } });
  }
  if (s.timeOver) {
    rules.push({ id: RULE_ID.time, priority: 200, action: page('time'), condition: { resourceTypes: ['main_frame'], excludedRequestDomains: [...LOCAL, ...learning] } });
    (s.learningPaths || []).slice(0, 50).forEach((p, i) => {
      rules.push({ id: RULE_ID.learnPath + i, priority: 210, action: { type: 'allow' }, condition: { urlFilter: `||${p.host}${p.path}`, resourceTypes: ['main_frame'] } });
    });
  }
  const block = (s.block || []).filter((d) => !LOCAL.includes(d));
  if (block.length) {
    rules.push({ id: RULE_ID.blockMain, priority: 160, action: page('parent'), condition: { resourceTypes: ['main_frame'], requestDomains: block } });
    rules.push({ id: RULE_ID.blockSub, priority: 160, action: { type: 'block' }, condition: { resourceTypes: NOT_MAIN, requestDomains: block } });
  }
  if (s.videoOver) {
    const vids = VIDEO_DOMAINS.filter((d) => !inDomains(d, learning));
    rules.push({ id: RULE_ID.videoMain, priority: 150, action: page('video'), condition: { resourceTypes: ['main_frame'], requestDomains: vids } });
    rules.push({ id: RULE_ID.videoFrame, priority: 150, action: { type: 'block' }, condition: { resourceTypes: ['sub_frame', 'media'], requestDomains: vids } });
  }
  const allow = (s.allow || []).filter((d) => !block.includes(d));
  if (allow.length) {
    rules.push({ id: RULE_ID.allow, priority: 100, action: { type: 'allow' }, condition: { resourceTypes: ALL_TYPES, requestDomains: allow } });
  }
  const content = (s.content || []).filter((d) => !inDomains(d, allow));
  if (content.length) {
    rules.push({ id: RULE_ID.content, priority: 90, action: page('content'), condition: { resourceTypes: ['main_frame'], requestDomains: content } });
  }
  const urls = (s.contentUrls || []).filter((k) => plainUrl(k) && !inDomains(hostOfKey(k), allow)).slice(-MAX_URL_RULES);
  urls.forEach((k, i) => {
    rules.push({ id: RULE_ID.contentUrl + i, priority: 90, action: page('content'), condition: { urlFilter: `|${k}^`, resourceTypes: ['main_frame'] } });
  });
  return rules;
}

const hostOfKey = (k) => { try { return new URL(k).hostname.replace(/^www\./, ''); } catch { return null; } };

/** The block reason for an open tab under state s, or null (mirrors buildRules for main frames). */
export function reasonFor(host, s, url) {
  if (!host || isLocalHost(host)) return null;
  if (s.bedtime) return 'bedtime';
  const learn = { hosts: s.learning || [], paths: s.learningPaths || [] };
  if (s.timeOver && !inDomains(host, learn.hosts) && !(url && isLearningUrl(url, learn))) return 'time';
  if (inDomains(host, s.block)) return 'parent';
  if (s.videoOver && inDomains(host, VIDEO_DOMAINS) && !inDomains(host, s.learning)) return 'video';
  if (inDomains(host, s.allow)) return null;
  if (inDomains(host, s.content)) return 'content';
  if (url && (s.contentUrls || []).includes(urlKey(url))) return 'content';
  return null;
}

/**
 * What the page check remembers for the day. A label (RTA, adult rating meta) blocks the whole host,
 * except learning hosts and big mixed hosts (wikipedia, google, youtube...); word evidence alone blocks
 * only that page (origin + path).
 */
export function contentScope(host, why, learning) {
  const L = learning || { hosts: [], paths: [] };
  const learnHost = inDomains(host, L.hosts || []) || (L.paths || []).some((p) => host === p.host || host.endsWith('.' + p.host));
  return (why === 'label' || why === 'rating') && !learnHost && !isMixedHost(host) ? 'host' : 'url';
}

export function blockedUrl(api, reason, host) {
  const q = new URLSearchParams({ reason });
  if (host) q.set('site', host);
  return api.runtime.getURL(`pages/blocked.html?${q}`);
}

let lastKey = null;

/** Replace our dynamic rules when they changed. Returns true when updated. */
export async function applyRules(api, s) {
  const rules = buildRules(s);
  const key = JSON.stringify(rules);
  if (key === lastKey) return false;
  const old = await api.declarativeNetRequest.getDynamicRules();
  await api.declarativeNetRequest.updateDynamicRules({ removeRuleIds: old.map((r) => r.id), addRules: rules });
  lastKey = key;
  return true;
}

/** Send open tabs that are now blocked to the block page. */
export async function sweepTabs(api, s, hostOf, onRedirect) {
  let tabs = [];
  try { tabs = await api.tabs.query({}); } catch { return; }
  for (const t of tabs) {
    const host = t.url ? hostOf(t.url) : null;
    const reason = reasonFor(host, s, t.url);
    if (!reason) continue;
    try {
      await api.tabs.update(t.id, { url: blockedUrl(api, reason, host) });
      if (onRedirect) onRedirect(t.id, t.url, reason);
    } catch { /* tab closed */ }
  }
}
