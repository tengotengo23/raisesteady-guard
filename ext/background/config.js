// Configuration: managed policy first (written by the Windows agent), else standalone settings
// from storage.local; band defaults from data/age-bands.json. resolveConfig() is pure (unit-tested).

import { DEFAULT_AGENT_PORT } from '../ui/brand.js';
import { normBedtime } from './schedule.js';
import { cleanDomains } from './domains.js';

export const LANGS = ['en', 'ka', 'ru']; // structure ready for es and zh: add the files, then the code here
export const DEFAULT_BAND = '9-12';      // used until a parent picks an age
export const SETTINGS_KEY = 'kg.settings';

export function pickLang(uiLang) {
  const l = String(uiLang || 'en').toLowerCase().split(/[-_]/)[0];
  return LANGS.includes(l) ? l : 'en';
}

export function bandById(bands, id) {
  const list = Array.isArray(bands) ? bands : [];
  return list.find((b) => b.id === id) || list.find((b) => b.id === DEFAULT_BAND) || list[0] || null;
}

const isObj = (v) => v && typeof v === 'object' && !Array.isArray(v);
// null = "no limit"; undefined = not set here
const num = (v) => {
  if (v === null) return null;
  if (v === undefined || v === '' || typeof v === 'boolean' || !Number.isFinite(Number(v))) return undefined;
  return Math.max(0, Math.round(Number(v)));
};
const pick = (...vals) => { for (const v of vals) if (v !== undefined) return v; return undefined; };
// Firefox gives managed booleans from the Windows registry as 1/0; accept those and "true"/"false" too.
const bool = (v) => {
  if (typeof v === 'boolean') return v;
  if (v === 1 || v === '1' || v === 'true') return true;
  if (v === 0 || v === '0' || v === 'false') return false;
  return undefined;
};

/** Managed mode = the policy carries an agent token or an age band. */
export function isManaged(managed) {
  return isObj(managed) && (typeof managed.agentToken === 'string' && managed.agentToken.length > 0 || typeof managed.ageBand === 'string');
}

/**
 * Precedence, field by field:
 *  managed:    agent state (the parent's live settings) > managed policy > band defaults
 *  standalone: storage.local settings > band defaults
 * `localExtraMin` = minutes a parent granted today in standalone mode.
 */
export function resolveConfig({ bands, managed, local, agent, uiLang, localExtraMin = 0 }) {
  const m = isManaged(managed) ? managed : null;
  const s = isObj(local) ? local : {};
  const a = m && isObj(agent) ? agent : null;
  const ageBand = (m ? pick(a?.band, m.ageBand) : s.ageBand) || DEFAULT_BAND;
  const bandEntry = bandById(bands, ageBand) || { id: ageBand };
  const src = m ? (a?.limits || {}) : (s.limits || {});
  const limits = {
    browseMin: pick(num(src.browseMin), bandEntry.browseMin ?? null),
    videoMin: pick(num(src.videoMin), bandEntry.videoMin ?? null),
  };
  const btSrc = m ? a?.bedtime : s.bedtime;
  const bedtime = btSrc === null || btSrc === false ? null : (normBedtime(btSrc) || normBedtime(bandEntry.bedtime));
  const focusMode = pick(bool(m ? a?.focusMode : s.focusMode), bool(m?.focusMode), true);
  const adblock = pick(bool(m ? a?.adblock : s.adblock), bool(m?.adblock), true);
  // Topic filter (P2-161): on unless the parent switched it off (old agents and settings: on).
  const topics = pick(bool(m ? a?.topics : s.topics), bool(m?.topics), true);
  return {
    ageBand: bandEntry.id || ageBand,
    band: { ...bandEntry, browseMin: limits.browseMin, videoMin: limits.videoMin, bedtime },
    focusMode, adblock, topics,
    standalone: !m,
    lang: pickLang(uiLang),
    limits, bedtime,
    extraMin: m ? (num(a?.extraMin) || 0) : (num(localExtraMin) || 0),
    allow: cleanDomains(m ? a?.allow : s.allow),
    block: cleanDomains(m ? a?.block : s.block),
    agentPort: m ? (Number.isInteger(m.agentPort) && m.agentPort > 0 && m.agentPort < 65536 ? m.agentPort : DEFAULT_AGENT_PORT) : null,
    agentToken: m && typeof m.agentToken === 'string' ? m.agentToken : null,
    installId: m && typeof m.installId === 'string' ? m.installId : null,
    setupDone: m ? true : !!(s.ageBand && s.pin),
    hasPin: m ? true : !!s.pin,
  };
}

/** What content scripts and pages may see (never the agent token). Shape fixed by the brief. */
export function publicConfig(cfg) {
  return { ageBand: cfg.ageBand, band: cfg.band, focusMode: cfg.focusMode, adblock: cfg.adblock, topics: cfg.topics !== false, standalone: cfg.standalone, lang: cfg.lang };
}

// ---- runtime loaders (need the browser API) ----

export async function readManaged(api) {
  try {
    const v = await api.storage.managed.get(null);
    return isObj(v) ? v : null;
  } catch { return null; } // Firefox throws when no policy is set
}

export async function readLocalSettings(api) {
  try { return (await api.storage.local.get(SETTINGS_KEY))[SETTINGS_KEY] || {}; } catch { return {}; }
}

export async function loadJSON(api, path, fallback) {
  try {
    const r = await fetch(api.runtime.getURL(path));
    if (!r.ok) return fallback;
    return await r.json();
  } catch { return fallback; }
}
