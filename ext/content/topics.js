// Topic filter (P2-161, T1): "Sexuality and gender identity topics", on by default under 18.
// Text only, on the device. Classic script: defines globalThis.KGTopics; also loads in node (tests,
// tools/update-videos.mjs) and in the background (side-effect import). The words live in data/topics.json.
//
// Rules (keep false positives low):
//  - a search, a title (page title, og:title, a video title) or the address path: one weight-2 term blocks;
//  - description, keywords and headings (h1, h2): weights add up, a page is blocked from 4
//    (two clear terms, or one clear term in two places); body text is never read;
//  - a term's "unless" pattern anywhere in the same text cancels it (other meanings: biology, names);
//  - sites on the domain list are blocked; helplines are never blocked;
//  - common spelling tricks are undone first (see fold): gaps (l g b t, g.a.y), digits (g4y), look-alike
//    letters mixing Latin and Cyrillic, repeated letters, hashtags. No AI, no model, no network: fixed rules only.
(() => {
  'use strict';
  if (globalThis.KGTopics) return;
  const B = '(?<![\\p{L}\\p{N}])(?:';
  // A word may be followed by digits (lgbtqia2s+, gay2026), not by more letters.
  const E = ')(?!\\p{L})';
  const LETTERS = '\\p{L}*';
  const PAGE_BLOCK = 4;
  // Inside a phrase a space or a dash may also be "_", "." or nothing (hashtags: #pridemonth, #trans_rights).
  const src = (p) => String(p).split('[- ]').join('\u0000').split(' ').join('\u0000').split('\u0000').join('[ _.-]?').split('{L}').join(LETTERS);

  // ---- spelling tricks (fixed rules, no guessing) ----
  // Look-alike letters, only inside a word that mixes Latin and Cyrillic (gаy with a Cyrillic а, гeй with a Latin e).
  const CYR2LAT = { 'а': 'a', 'в': 'b', 'е': 'e', 'ё': 'e', 'к': 'k', 'м': 'm', 'н': 'h', 'о': 'o', 'р': 'p', 'с': 'c', 'т': 't', 'у': 'y', 'х': 'x', 'і': 'i', 'ј': 'j', 'ѕ': 's', 'ԁ': 'd', 'ɡ': 'g', 'ӏ': 'l' };
  const LAT2CYR = { 'a': 'а', 'b': 'в', 'e': 'е', 'k': 'к', 'm': 'м', 'h': 'н', 'o': 'о', 'p': 'р', 'c': 'с', 't': 'т', 'y': 'у', 'x': 'х', 'i': 'і', 'l': 'л', 'g': 'г', 'n': 'п', 'u': 'и' };
  const LATIN = /[a-z]/, CYR = /[\u0400-\u04ff]/;
  // Digits and signs for letters, only inside a word that also has letters (g4y, l3sbian, tr@ns, $ex).
  const LEET = { '0': 'o', '1': 'i', '3': 'e', '4': 'a', '5': 's', '7': 't', '@': 'a', '$': 's', '!': 'i', '|': 'l' };
  function fold(s, one, rep) {
    let x = s;
    // l33t inside mixed tokens
    x = x.replace(/[\p{L}\p{N}@$!|]+/gu, (w) => (/\p{L}/u.test(w) && /[0-9@$!|]/.test(w) ? w.replace(/[013457@$!|]/g, (c) => (c === '1' ? one : LEET[c])) : w));
    // dots, stars, underscores, apostrophes and dashes inside a word: tran.sgender, l*sbian, bi-sexual
    x = x.replace(/(?<=\p{L})[.*_·'-]+(?=\p{L})/gu, '');
    // single letters with gaps (also emoji): l g b t, g.a.y, q-u-e-e-r, g😀a😀y
    x = x.replace(/(?<![\p{L}\p{N}])(?:\p{L}(?:[ ._*·~+\/-]|\p{Extended_Pictographic}|\ufe0f)+){2,}\p{L}(?![\p{L}\p{N}])/gu, (m) => m.replace(/(?:[ ._*·~+\/-]|\p{Extended_Pictographic}|\ufe0f)+/gu, ''));
    // the same letter three times or more: gaaay, lgbtttt (once), queeeer (twice)
    x = x.replace(/(\p{L})\1{2,}/gu, rep);
    // look-alike letters inside mixed words, both ways
    const cyr = [], lat = [];
    x.replace(/\p{L}+/gu, (w) => { if (LATIN.test(w) && CYR.test(w)) { lat.push(w); } return w; });
    let toLat = x, toCyr = x;
    for (const w of lat) {
      toLat = toLat.split(w).join([...w].map((c) => CYR2LAT[c] || c).join(''));
      toCyr = toCyr.split(w).join([...w].map((c) => LAT2CYR[c] || c).join(''));
      cyr.push(w);
    }
    return cyr.length ? [toLat, toCyr] : [x];
  }
  /** The text as written, and the text with the spelling tricks undone (1 read as i, and as l). */
  function variants(text) {
    const s = norm(text);
    if (!s) return [];
    return [...new Set([s, ...fold(s, 'i', '$1'), ...fold(s, 'l', '$1'), ...fold(s, 'i', '$1$1')])];
  }

  /** Lower case, invisible characters gone (zero-width, soft hyphen), full-width letters made plain (NFKC),
   *  one kind of apostrophe and dash, spaces folded. */
  function norm(s) {
    return String(s ?? '').replace(/[\u00ad\u034f\u061c\u115f\u1160\u17b4\u17b5\u180e\u200b-\u200f\u202a-\u202e\u2060-\u2064\u206a-\u206f\u3164\ufeff\uffa0]/g, '')
      .normalize('NFKC').toLowerCase()
      .replace(/[‘’ʼ]/g, "'").replace(/[‐-―]/g, '-').replace(/\s+/g, ' ').trim();
  }
  /** The address path as words: /news/lgbt-history_month.html -> "news lgbt history month html". */
  function pathWords(path) {
    let p = String(path || '');
    try { p = decodeURIComponent(p); } catch { /* keep as is */ }
    return norm(p.replace(/[/_+.=&?#%-]+/g, ' '));
  }
  const hostIn = (host, list) => {
    const h = String(host || '').toLowerCase().replace(/^www\./, '').replace(/\.$/, '');
    return !!h && list.some((d) => h === d || h.endsWith('.' + d));
  };

  function create(data) {
    const d = data && typeof data === 'object' ? data : {};
    const terms = [];
    for (const t of Array.isArray(d.terms) ? d.terms : []) {
      if (!Array.isArray(t) || typeof t[0] !== 'string' || typeof t[1] !== 'string') continue;
      try {
        terms.push({ id: t[0], w: Number(t[2]) === 1 ? 1 : 2, re: new RegExp(B + src(t[1]) + E, 'u'), unless: t[3] ? new RegExp(B + src(t[3]) + E, 'u') : null });
      } catch { /* a broken pattern is skipped, never thrown */ }
    }
    const domains = (Array.isArray(d.domains) ? d.domains : []).map((x) => String(x).toLowerCase());
    const helplines = (Array.isArray(d.helplines) ? d.helplines : []).map((x) => String(x).toLowerCase());

    /** Terms found in one text: [{ id, w }]. */
    function find(text) {
      const vs = variants(text);
      const out = [];
      for (const t of terms) {
        if (vs.some((s) => t.re.test(s) && !(t.unless && t.unless.test(s)))) out.push({ id: t.id, w: t.w });
      }
      return out;
    }
    const strongIn = (text) => find(text).filter((x) => x.w >= 2);

    /** A search or a title: one clear term is enough. */
    function matchTitle(text) {
      const hits = strongIn(text);
      return { hit: hits.length > 0, terms: hits.map((x) => x.id) };
    }

    /**
     * A page: { host, path, title, ogTitle, description, ogDescription, keywords, headings: string[] }.
     * Returns { hit, why: 'helpline'|'domain'|'title'|'path'|'score'|null, terms, score }.
     */
    function matchPage(p) {
      const q = p || {};
      if (hostIn(q.host, helplines)) return { hit: false, why: 'helpline', terms: [], score: 0 };
      if (hostIn(q.host, domains)) return { hit: true, why: 'domain', terms: [], score: 0 };
      for (const [why, text] of [['title', q.title], ['title', q.ogTitle], ['path', pathWords(q.path)]]) {
        const m = matchTitle(text);
        if (m.hit) return { hit: true, why, terms: m.terms, score: 0 };
      }
      let score = 0;
      const seen = new Set();
      const heads = Array.isArray(q.headings) ? q.headings.slice(0, 20).join(' · ') : q.headings;
      for (const text of [[q.description, q.ogDescription].filter(Boolean).join(' · '), q.keywords, heads]) {
        for (const x of find(text)) { score += x.w; seen.add(x.id); }
      }
      return { hit: score >= PAGE_BLOCK, why: score >= PAGE_BLOCK ? 'score' : null, terms: [...seen], score };
    }

    return {
      matchQuery: matchTitle, matchTitle, matchPage,
      isHelpline: (host) => hostIn(host, helplines),
      isTopicDomain: (host) => hostIn(host, domains) && !hostIn(host, helplines),
      domains: () => domains.slice(), helplines: () => helplines.slice(),
      helpLink: typeof d.helpLink === 'string' && /^https:\/\//.test(d.helpLink) ? d.helpLink : 'https://findahelpline.com/',
      size: terms.length,
    };
  }

  globalThis.KGTopics = { create, norm, pathWords, variants, PAGE_BLOCK };
})();
