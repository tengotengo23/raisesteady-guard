// Our own small i18n: loads i18n/<area>.<lang>.json, falls back to English per key.
// Language = the browser UI language when we have it, else en. Works in pages and the background.

import { APP_NAME } from './brand.js';

export const LANGS = ['en', 'ka', 'ru']; // add 'es', 'zh' here once their files exist

const api = globalThis.browser ?? globalThis.chrome;

export function uiLang() {
  let l = 'en';
  try { l = api.i18n.getUILanguage(); } catch { l = (globalThis.navigator && navigator.language) || 'en'; }
  const short = String(l).toLowerCase().split(/[-_]/)[0];
  return LANGS.includes(short) ? short : 'en';
}

async function load(area, lang) {
  try {
    const r = await fetch(api.runtime.getURL(`i18n/${area}.${lang}.json`));
    return r.ok ? await r.json() : {};
  } catch { return {}; }
}

/** Returns t(key, vars). {name} in a string is replaced by vars.name; {app} by the display name. */
export async function loadI18n(area = 'core', lang = uiLang()) {
  const [en, own] = await Promise.all([load(area, 'en'), lang === 'en' ? null : load(area, lang)]);
  const dict = { ...en, ...(own || {}) };
  const t = (key, vars = {}) => {
    const s = dict[key] ?? key;
    return String(s).replace(/\{(\w+)\}/g, (m, k) => (k === 'app' ? APP_NAME : vars[k] != null ? String(vars[k]) : m));
  };
  t.lang = lang;
  t.has = (key) => key in dict;
  return t;
}

/** Fill [data-i18n] text, [data-i18n-attr="placeholder:key;aria-label:key"] and the page lang. */
export function applyI18n(t, root = document) {
  for (const el of root.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n);
  for (const el of root.querySelectorAll('[data-i18n-attr]')) {
    for (const pair of el.dataset.i18nAttr.split(';')) {
      const [attr, key] = pair.split(':').map((x) => x.trim());
      if (attr && key) el.setAttribute(attr, t(key));
    }
  }
  if (root === document) document.documentElement.lang = t.lang;
}

/** "1 h 5 min" / "45 min" in the page language. */
export function fmtMinutes(t, sec) {
  const m = Math.max(0, Math.ceil(sec / 60));
  if (m < 60) return t('minShort', { n: m });
  const h = Math.floor(m / 60), r = m % 60;
  return r ? t('hourMinShort', { h, m: r }) : t('hourShort', { h });
}
