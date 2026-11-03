// Ad blocker switch. Three layers follow config.adblock:
//  1. network rules: the "ads" (EasyList/EasyPrivacy domains) and "yt_ads" (ours) static rulesets;
//     core's applyRulesets() turns them on and off with the same switch; we re-check here so the
//     switch holds even if the core changes.
//  2. cosmetic CSS (rules/cosmetic.css): every rule sits under html:not([data-kg-ads="off"]);
//     content/cosmetic.js sets that attribute from kg.b.adblock, which we write here.
//  3. YouTube player data (content/yt-ads-main.js, MAIN world): reads data-kg-ads on <html> the same way.
// Messages: kg:ads:status -> { adblock }.

const KEY = 'kg.b.adblock';
const RULESETS = ['ads', 'yt_ads'];

export function initAdblock(ctx) {
  const api = ctx.api;
  let last = null;
  const manifestIds = (() => {
    try { return ((api.runtime.getManifest().declarative_net_request || {}).rule_resources || []).map((r) => r.id); } catch { return []; }
  })();

  async function apply(cfg) {
    if (!cfg) return;
    const on = cfg.adblock !== false;
    if (on !== last) {
      last = on;
      await api.storage.local.set({ [KEY]: on }).catch(() => {});
    }
    try {
      const enabled = await api.declarativeNetRequest.getEnabledRulesets();
      const ids = RULESETS.filter((id) => manifestIds.includes(id));
      const enable = on ? ids.filter((id) => !enabled.includes(id)) : [];
      const disable = on ? [] : ids.filter((id) => enabled.includes(id));
      if (enable.length || disable.length) await api.declarativeNetRequest.updateEnabledRulesets({ enableRulesetIds: enable, disableRulesetIds: disable });
    } catch { /* core manages rulesets too */ }
  }

  ctx.onConfig((cfg) => { apply(cfg).catch(() => {}); });

  api.runtime.onMessage.addListener((msg, sender, sendResponse) => {
    if (!msg || typeof msg.type !== 'string' || !msg.type.startsWith('kg:ads:')) return false;
    if (sender.id !== api.runtime.id) return false;
    (async () => {
      for (let i = 0; i < 50 && !ctx.ready; i++) await new Promise((r) => setTimeout(r, 20));
      try { await ctx.ready; } catch { /* defaults */ }
      const cfg = ctx.getConfig();
      if (msg.type === 'kg:ads:status') return { adblock: !cfg || cfg.adblock !== false };
      return { error: 'unknown' };
    })().then(sendResponse, () => sendResponse({ adblock: true }));
    return true;
  });
}
