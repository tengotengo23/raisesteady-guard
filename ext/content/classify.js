// Page check classifier (local rules only, no network). Classic script: defines globalThis.KGClassify.
// Blocks only on strong evidence: the RTA label, an adult rating meta tag, or at least three different
// strong terms with a high weighted score. Health, sex-education, art and news pages must pass, so
// medical and educational words are never strong terms, and pages with clear safe context need more.
(() => {
  'use strict';
  if (globalThis.KGClassify) return;

  const RTA = 'rta-5042-1996-1400-1577-rta';
  const L = '\\p{L}*';
  // [root id, pattern, weight] weight 3 = strong, 1.5 = medium, 0.5 = weak (score only)
  const TERMS = [
    // English
    ['porn', `porn${L}`, 3], ['xxx', 'xxx+', 3], ['milf', 'milfs?', 3], ['blowjob', 'blow ?jobs?', 3],
    ['cumshot', 'cum ?shots?', 3], ['gangbang', 'gang ?bang' + L, 3], ['creampie', 'creampies?', 3],
    ['deepthroat', 'deepthroat' + L, 3], ['handjob', 'hand ?jobs?', 3], ['bukkake', 'bukkake', 3],
    ['hentai', 'hentai', 3], ['camgirl', 'cam ?girls?|webcam sex|sex ?cams?|live sex', 3], ['nsfw', 'nsfw', 3],
    ['sexvideo', 'sex (?:videos?|movies?|tube|chat|tapes?|clips?)', 3],
    ['nudegirls', 'nude (?:girls?|women|teens?|pics|photos|selfies|videos?)|naked (?:girls?|women|teens?)', 3],
    ['bigparts', 'big (?:tits|boobs|cocks?|dicks?)', 3], ['slut', `sluts?|slutty`, 3],
    ['brand', 'xvideos|xhamster|redtube|youporn|brazzers|chaturbate|stripchat|spankbang|xnxx|bongacams|livejasmin', 3],
    ['hardcore', 'hardcore', 1.5], ['pussy', 'pussy|pussies', 1.5], ['horny', 'horny', 1.5], ['threesome', 'threesomes?', 1.5],
    ['erotic', `erotic${L}|erotica`, 1.5], ['escort', 'escorts? (?:service|girls?|agency|agencies)', 1.5],
    ['boobs', 'boobs?', 1.5], ['fuck', `fuck${L}`, 1.5], ['stripper', 'strippers?|striptease', 1.5],
    ['onlyfans', 'onlyfans', 1.5], ['adultonly', 'adult (?:videos?|movies?|content|site|entertainment)|you must be (?:18|eighteen)', 1.5],
    ['nudes', 'nudes', 1.5],
    ['fetish', `fetish${L}`, 0.5], ['tits', 'tits', 0.5], ['naked', 'naked', 0.5], ['orgasm', `orgasm${L}`, 0.5],
    ['masturb', `masturbat${L}`, 0.5], ['18plus', '18\\+', 0.5],
    // Georgian
    ['ka-porn', `პორნ${L}`, 3], ['ka-sexvideo', `სექს ?ვიდეო${L}|სექს ?ჩატ${L}|სექსი ონლაინ`, 3],
    ['ka-boz', `ბოზ${L}`, 3], ['ka-yle', `ყლე${L}`, 3], ['ka-mutel', `მუტელ${L}`, 3],
    ['ka-erotic', `ეროტიკ${L}|ეროტიულ${L}`, 1.5], ['ka-escort', `ესკორტ${L}`, 1.5], ['ka-trak', `ტრაკ${L}`, 1.5],
    ['ka-naked', `შიშველ${L}`, 0.5], ['ka-sex', 'სექსი', 0.5], ['ka-intim', `ინტიმ${L}`, 0.5], ['ka-masturb', `მასტურბაც${L}`, 0.5],
    // Russian
    ['ru-porn', `порн${L}`, 3], ['ru-sexvideo', `секс[- ]?(?:видео|чат${L}|знакомств${L}|кам${L})`, 3],
    ['ru-minet', `минет${L}`, 3], ['ru-shluh', `шлюх${L}`, 3], ['ru-sisk', `сиськ${L}|сисек`, 3],
    ['ru-trah', `трахн${L}|трахает${L}|трахать${L}|трахают${L}|трахал${L}`, 3],
    ['ru-ebl', `ебл${L}|ебут${L}|ебать${L}|выеб${L}|отъеб${L}`, 3],
    ['ru-naked', `голы[еймх]${L} (?:девуш${L}|девоч${L}|женщин${L}|баб${L}|телочк${L})`, 3],
    ['ru-hentai', 'хентай', 3], ['ru-milf', `милф${L}`, 3], ['ru-cuckold', `куколд${L}`, 3], ['ru-cumshot', `камшот${L}`, 3],
    ['ru-droch', `дроч${L}`, 3], ['ru-webcam', `вебкам${L}`, 3],
    ['ru-erotic', `эроти${L}`, 1.5], ['ru-hui', `хуй|хуя|хуё${L}|хуе[вст]${L}`, 1.5], ['ru-pizd', `пизд${L}`, 1.5],
    ['ru-indiv', `индивидуалк${L}`, 1.5], ['ru-adult', 'для взрослых', 1.5],
    ['ru-prostit', `проститут${L}`, 0.5], ['ru-intim', `интим${L}`, 0.5], ['ru-sex', 'секс', 0.5],
  ];
  const SAFE = [
    // en
    `health${L}`, `medic${L}`, `doctors?`, `clinic${L}`, `hospital${L}`, `patients?`, `symptoms?`, `treatment${L}`,
    `educat${L}`, `schools?`, `teachers?`, `curricul${L}`, 'puberty', `anatom${L}`, `biolog${L}`, `reproduct${L}`,
    `contracept${L}`, 'hiv', `cancer${L}`, `screening${L}`, `mammogra${L}`, `research${L}`, `universit${L}`,
    `museum${L}`, `galler${L}`, `painting${L}`, `sculpture${L}`, 'renaissance', 'art history', 'police', `arrest${L}`,
    'court', `charged`, `sentenced`, `prosecut${L}`, `legislat${L}`, `investigat${L}`, 'wikipedia', `encyclop${L}`,
    `parents?`, `consent`, `safeguard${L}`,
    // ka
    `ჯანმრთელობ${L}`, `ექიმ${L}`, `კლინიკ${L}`, `საავადმყოფო${L}`, `განათლებ${L}`, `სკოლ${L}`, `მასწავლებ${L}`,
    `მომწიფებ${L}`, `ანატომი${L}`, `ბიოლოგი${L}`, `კიბო${L}`, `მუზეუმ${L}`, `ხელოვნებ${L}`, `ფერწერ${L}`, `ქანდაკებ${L}`,
    `პოლიცი${L}`, `დააკავ${L}`, `სასამართლო${L}`, `გამოძიებ${L}`, `კანონ${L}`, `მშობ${L}`, `უსაფრთხოებ${L}`, `კვლევ${L}`,
    // ru
    `здоров${L}`, `медицин${L}`, `врач${L}`, `клиник${L}`, `больниц${L}`, `пациент${L}`, `симптом${L}`, `лечени${L}`,
    `образован${L}`, `школ${L}`, `учител${L}`, `созревани${L}`, `анатоми${L}`, `биологи${L}`, 'рак', 'рака', 'раком',
    `маммограф${L}`, `исследован${L}`, `университет${L}`, `музе${L}`, `живопис${L}`, `скульптур${L}`, `искусств${L}`,
    `полици${L}`, `задержа${L}`, 'суд', 'суда', 'судом', `обвин${L}`, `закон${L}`, `расследовани${L}`, `родител${L}`, `безопасност${L}`,
  ];
  const URL_RE = /porn|xxx|xvideo|xhamster|hentai|sex(?:cam|chat|tube|video)|порн|პორნ/i;
  const B = '(?<![\\p{L}\\p{N}])(?:';
  const E = ')(?![\\p{L}\\p{N}])';
  const termRes = TERMS.map(([id, p, w]) => ({ id, w, re: new RegExp(B + p + E, 'gu') }));
  const safeRes = SAFE.map((p) => new RegExp(B + p + E, 'u'));
  const FIELD_W = { title: 3, description: 2, keywords: 1, headings: 2, text: 1 };
  const MAX_COUNT = 5;

  const norm = (s) => String(s || '').toLowerCase().replace(/\s+/g, ' ');

  /**
   * @param {object} p { title, description, keywords, headings: string|string[], text, url, metas: [{name, content}] }
   * @returns {{block: boolean, reason: string|null, score: number, strong: string[], safe: number}}
   */
  function classify(p) {
    const metas = Array.isArray(p.metas) ? p.metas : [];
    for (const m of metas) {
      const c = norm(m.content);
      if (c.includes(RTA)) return { block: true, reason: 'label', score: 100, strong: ['rta'], safe: 0 };
    }
    for (const m of metas) {
      const n = norm(m.name);
      const c = norm(m.content).trim();
      if (n === 'rating' && (/^(adult|mature|rta\b|restricted|18\+?)/.test(c))) return { block: true, reason: 'rating', score: 100, strong: ['rating'], safe: 0 };
    }
    const fields = {
      title: norm(p.title),
      description: norm(p.description),
      keywords: norm(p.keywords),
      headings: norm(Array.isArray(p.headings) ? p.headings.join(' · ') : p.headings),
      text: norm(p.text).slice(0, 20000),
    };
    let score = 0;
    const strong = new Set(), medium = new Set();
    for (const t of termRes) {
      let hit = 0;
      for (const [f, fw] of Object.entries(FIELD_W)) {
        const s = fields[f];
        if (!s) continue;
        t.re.lastIndex = 0;
        let n = 0;
        while (n < MAX_COUNT && t.re.exec(s)) n++;
        if (n) { score += t.w * n * fw; hit += n; }
      }
      if (hit) { if (t.w >= 3) strong.add(t.id); else if (t.w >= 1.5) medium.add(t.id); }
    }
    if (p.url && URL_RE.test(String(p.url))) { strong.add('url'); score += 6; }
    const all = Object.values(fields).join(' ');
    let safe = 0;
    for (const re of safeRes) if (re.test(all)) safe++;
    const distinct = strong.size + Math.floor(medium.size / 2);
    const need = safe >= 4 ? { distinct: 5, score: 60 } : { distinct: 3, score: 24 };
    const block = distinct >= need.distinct && score >= need.score;
    return { block, reason: block ? 'words' : null, score: Math.round(score * 10) / 10, strong: [...strong], safe };
  }

  globalThis.KGClassify = { classify, RTA };
})();
