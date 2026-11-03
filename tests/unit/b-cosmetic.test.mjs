// EasyList element-hiding conversion (tools/build-cosmetic.mjs) and the built rules/cosmetic.css.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, statSync } from 'node:fs';
import { parseHiding, parseGenericHide, buildCss, safeSelector, baseDomain, EASYLIST_COMMIT } from '../../tools/build-cosmetic.mjs';

const SAMPLE = `[Adblock Plus 2.0]
! comment
##.ad-banner
###AD_300
##div[id^="div-gpt-ad"]
~example.org##.sponsor-box
news.example.com,~shop.news.example.com##.promo
youtube.com##ytd-ad-slot-renderer
example.net#@#.ad-banner
site.example#?#.x:-abp-has(.y)
site.example##.a:has-text(Ad)
site.example#$#abort-on-property-read x
bad.example##.x { color: red }
bad.example##html > body .y
amazon.*##.ad
##.ok:has(> .ad)
`;

test('safeSelector refuses rule breaks, comments, extended syntax and unbalanced input', () => {
  for (const s of ['.ad', '#AD_300', 'div[id^="div-gpt-ad"]', '.ok:has(> .ad)', 'a[href*="}"]']) assert.equal(safeSelector(s), !s.includes('}'), s);
  for (const s of ['.x { color: red }', '.a /* x */', '.a:has-text(x)', '.a:-abp-has(.b)', 'div[', 'a)', 'html > body .y', '.a\\', '.a</style>']) assert.equal(safeSelector(s), false, s);
});

test('parses generic, specific, exceptions and skips non-CSS rules', () => {
  const h = parseHiding(SAMPLE);
  assert.ok(h.generic.has('.ad-banner') && h.generic.has('#AD_300') && h.generic.has('div[id^="div-gpt-ad"]') && h.generic.has('.ok:has(> .ad)'));
  assert.ok(h.generic.has('.sponsor-box'));
  assert.deepEqual([...h.exceptions.get('.sponsor-box')], ['example.org']);
  assert.deepEqual([...h.exceptions.get('.ad-banner')], ['example.net']);
  assert.deepEqual(h.specific.map((r) => [r.sel, r.include, r.exclude]), [
    ['.promo', ['news.example.com'], ['shop.news.example.com']],
    ['ytd-ad-slot-renderer', ['youtube.com'], []],
  ]);
  assert.equal(h.skipped, 6);
});

test('generichide sites from the allowlist (whole-site rules only)', () => {
  const gh = parseGenericHide(`@@||whole.example^$generichide\n@@||path.example/x$generichide\n@@||elem.example^$elemhide,document\n@@||scoped.example^$generichide,domain=a.com\n@@://localhost/$generichide\n@@||nothing.example^$image`);
  assert.deepEqual([...gh].sort(), ['elem.example', 'whole.example']);
});

test('builds gated, nested CSS that honours exceptions and stays under budget', () => {
  const h = parseHiding(SAMPLE);
  const out = buildCss({ hiding: h, genericHide: new Set(['whole.example']), topSites: new Map([['example.com', 10]]), alwaysSites: ['youtube.com'], header: '/* h */\n' });
  const css = out.css;
  assert.ok(css.startsWith('/* h */\nhtml:not([data-kg-ads="off"]):not([data-kg-h~="whole.example"]) {'));
  assert.match(css, /&:not\(\[data-kg-h~="example\.net"\]\) \.ad-banner \{ display: none !important; \}/);
  assert.match(css, /&:not\(\[data-kg-h~="example\.org"\]\) \.sponsor-box/);
  assert.match(css, /&:is\(\[data-kg-h~="news\.example\.com"\]\):not\(\[data-kg-h~="shop\.news\.example\.com"\]\) \{\s+\.promo/);
  assert.match(css, /&:is\(\[data-kg-h~="youtube\.com"\]\) \{\s+ytd-ad-slot-renderer/);
  assert.ok(!/abp|has-text|color: red|amazon/.test(css));
  let depth = 0; for (const c of css.replace(/"[^"]*"/g, '')) { if (c === '{') depth++; if (c === '}') depth--; assert.ok(depth >= 0); }
  assert.equal(depth, 0);
  const tiny = buildCss({ hiding: h, genericHide: new Set(), topSites: new Map([['example.com', 10]]), alwaysSites: [], maxBytes: 300 });
  assert.equal(tiny.stats.specificRules, 0); // over budget: site rules are dropped, generic stays
  assert.equal(baseDomain('www.bbc.co.uk'), 'bbc.co.uk');
  assert.equal(baseDomain('m.youtube.com'), 'youtube.com');
});

test('the built rules/cosmetic.css: pinned source, licence, gate, size under 400 KB', () => {
  const p = new URL('../../ext/rules/cosmetic.css', import.meta.url);
  const css = readFileSync(p, 'utf8');
  assert.ok(statSync(p).size < 400 * 1024, `${statSync(p).size} bytes`);
  assert.ok(css.includes(EASYLIST_COMMIT) && css.includes('CC BY-SA 3.0') && css.includes('The EasyList authors'));
  const tops = css.split('\n').filter((l) => /^html/.test(l));
  assert.ok(tops.length >= 2 && tops.every((l) => l.startsWith('html:not([data-kg-ads="off"])')));
  assert.ok(css.includes('#AD_300') && css.includes('.ad-slot'));
  assert.ok(css.includes('[data-kg-h~="youtube.com"]'));
  assert.ok(!/:-abp-|:has-text\(|:xpath\(|<\/style/i.test(css));
});
