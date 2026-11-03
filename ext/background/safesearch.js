// Safe search and YouTube Restricted Mode, done with static rulesets (rules/safesearch.json,
// rules/yt-strict.json, rules/yt-moderate.json), plus which static rulesets are on for the band.

/** Which static rulesets should be on. Pure (unit-tested). */
export function wantedRulesets(cfg) {
  return {
    adult: true, gambling: true, bypass: true, safesearch: true,
    social: cfg.band.social !== 'feedfree',
    ads: !!cfg.adblock, yt_ads: !!cfg.adblock,
    yt_strict: cfg.band.youtube !== 'moderate',
    yt_moderate: cfg.band.youtube === 'moderate',
  };
}

/** A Google search URL with SafeSearch on (the rules add it anyway). */
export function safeSearchUrl(q) {
  return 'https://www.google.com/search?' + new URLSearchParams({ q, safe: 'active' });
}

/** Enable and disable the static rulesets for the band and switches. */
export async function applyRulesets(api, cfg, manifestIds) {
  const want = wantedRulesets(cfg);
  let enabled = [];
  try { enabled = await api.declarativeNetRequest.getEnabledRulesets(); } catch { return; }
  const enable = [], disable = [];
  for (const [id, on] of Object.entries(want)) {
    if (!manifestIds.includes(id)) continue;
    if (on && !enabled.includes(id)) enable.push(id);
    if (!on && enabled.includes(id)) disable.push(id);
  }
  if (enable.length || disable.length) {
    await api.declarativeNetRequest.updateEnabledRulesets({ enableRulesetIds: enable, disableRulesetIds: disable });
  }
}
