// Topic filter in the background (P2-161, T1). The matcher is content/topics.js (globalThis.KGTopics).
// topicSearch() is pure (unit-tested); topicsStore() hands the switch and the word list to the page checks
// through storage.local (they read it at page load without waking the background).
import { searchQuery } from './searches.js';
import { inDomains } from './domains.js';

export const TOPICS_KEY = 'kg.topics';

/** Helplines are never sent to any block page, by any path (content check, topic check, video time, sweeps). */
export function helplineHost(host, matcher) {
  return !!host && !!matcher && typeof matcher.isHelpline === 'function' && matcher.isHelpline(host);
}

/** true when this address is a search (7 engines, YouTube too) on the topic and the filter applies. */
export function topicSearch(url, cfg, matcher, host) {
  if (!cfg || cfg.topics === false || !matcher) return false;
  if (host && (inDomains(host, cfg.allow || []) || helplineHost(host, matcher))) return false;
  const q = searchQuery(url);
  return !!q && matcher.matchQuery(q).hit;
}

let lastKey = null;
/** Write { on, allow, helplines, data } for the content scripts when it changed. The helplines go out even
 *  with the topic filter off: the page check never flags a helpline for any reason. */
export function topicsValue(cfg, data) {
  const on = !!cfg && cfg.topics !== false && !!data;
  return { on, allow: (cfg && cfg.allow) || [], helplines: (data && Array.isArray(data.helplines) ? data.helplines : []), data: on ? data : null };
}
export async function topicsStore(api, cfg, data) {
  const v = topicsValue(cfg, data);
  const key = JSON.stringify([v.on, v.allow, v.helplines, data && data.version, data && data.terms && data.terms.length]);
  if (key === lastKey) return false;
  await api.storage.local.set({ [TOPICS_KEY]: v });
  lastKey = key;
  return true;
}
