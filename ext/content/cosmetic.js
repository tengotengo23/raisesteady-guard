// Cosmetic ad hiding switch. rules/cosmetic.css is injected on every page, but every rule in it
// sits under html:not([data-kg-ads="off"]) and its site rules under [data-kg-h~="<site>"]. This script:
//  1. writes data-kg-h = the host and its parent domains ("www.bbc.co.uk bbc.co.uk co.uk uk"), so site rules
//     and EasyList's per-site exceptions apply;
//  2. writes data-kg-ads="off" while the parent has the ad blocker off (kg.b.adblock in storage.local,
//     written by background/adblock.js), and removes it when it is on again, live.
// Fails safe: if anything throws, the page simply keeps its default (blocker on, generic rules only).
(() => {
  try {
    const de = document.documentElement;
    if (!de || de.hasAttribute('data-kg-cos')) return;
    de.setAttribute('data-kg-cos', '');
    const host = String(location.hostname || '').toLowerCase().replace(/\.$/, '');
    const parts = host ? host.split('.') : [];
    const hosts = parts.map((_, i) => parts.slice(i).join('.')).join(' ');
    let off = false;
    const apply = () => {
      if (hosts && de.getAttribute('data-kg-h') !== hosts) de.setAttribute('data-kg-h', hosts);
      if (off && de.getAttribute('data-kg-ads') !== 'off') de.setAttribute('data-kg-ads', 'off');
      if (!off && de.hasAttribute('data-kg-ads')) de.removeAttribute('data-kg-ads');
    };
    apply();
    // Some pages rewrite <html> attributes while they start; put ours back.
    new MutationObserver(apply).observe(de, { attributes: true, attributeFilter: ['data-kg-h', 'data-kg-ads'] });
    const api = globalThis.browser ?? globalThis.chrome;
    const set = (v) => { off = v === false; apply(); };
    api.storage.local.get('kg.b.adblock').then((r) => set(r && r['kg.b.adblock'])).catch(() => {});
    api.storage.onChanged.addListener((c, area) => { if (area === 'local' && c['kg.b.adblock']) set(c['kg.b.adblock'].newValue); });
  } catch { /* never break the page */ }
})();
