// Feed-free mode on scroll sites. For 13 to 17 (under 13 these sites are blocked by the core's
// social ruleset; if a parent allows one, it is feed-free too). On a site's feed pages (home feed, Reels,
// Explore, For You) the feed is hidden and a small panel with that site's search takes its place, in the
// page flow. Search, profiles and messages keep working; sign-in pages (a password field) are left alone. Selectors live in one object per site below:
// sites change their layout often, so update them there.
(() => {
  'use strict';
  const de = document.documentElement;
  if (!de || de.hasAttribute('data-kg-ff')) return;
  de.setAttribute('data-kg-ff', '');
  const api = globalThis.browser ?? globalThis.chrome;
  const HOST_ID = 'kg-ff-panel';

  // feed: path patterns that are feeds (hidden). keep: patterns that look like feeds but must work (search).
  // main: where the feed lives (hidden on feed pages; the panel goes just before it).
  // hide: entries that lead to feeds (nav links), hidden on every page. search: the site's search address.
  const SITES = {
    'instagram.com': {
      name: 'Instagram', feed: [/^\/$/, /^\/explore\/?$/, /^\/explore\/(?!search)/, /^\/reels?\//],
      keep: [/^\/explore\/search/], main: ['main[role="main"]', 'main', 'section > main'],
      hide: ['a[href="/explore/"]', 'a[href^="/reels/"]', 'a[href="/reels"]'],
      search: (q) => `https://www.instagram.com/explore/search/keyword/?q=${q}`,
    },
    'facebook.com': {
      name: 'Facebook', feed: [/^\/$/, /^\/home\.php/, /^\/watch\/?/, /^\/reels?\//, /^\/gaming\/?$/, /^\/stories\//],
      main: ['[role="main"]', 'main'],
      hide: ['a[href*="/watch/"]', 'a[href*="/reel/"]', 'a[href*="/reels/"]', 'a[href*="/gaming/"]', '[aria-label="Stories"]', 'div[role="region"][aria-label="Stories"]'],
      search: (q) => `https://www.facebook.com/search/top/?q=${q}`,
    },
    'tiktok.com': {
      name: 'TikTok', feed: [/^\/$/, /^\/foryou/, /^\/shortdrama/, /^\/explore/, /^\/following\/?$/, /^\/friends\/?$/, /^\/live\/?$/, /^\/[a-z]{2}(-[A-Z]{2})?\/?$/],
      main: ['#main-content-homepage_hot', '#main-content-explore_page', '#main-content-video_detail', 'main', '[id^="main-content"]'],
      hide: ['a[href="/foryou"]', 'a[href^="/shortdrama"]', 'a[href^="/explore"]', 'a[href="/following"]', 'a[href="/friends"]', 'a[href^="/live"]', '[data-e2e="recommend-list-item-container"]'],
      search: (q) => `https://www.tiktok.com/search?q=${q}`,
    },
    'x.com': {
      name: 'X', feed: [/^\/home\/?$/, /^\/explore\/?/, /^\/i\/trends/, /^\/i\/communitynotes/],
      main: ['main[role="main"] [data-testid="primaryColumn"]', 'main[role="main"]', 'main'],
      hide: ['a[href="/explore"]', 'a[data-testid="AppTabBar_Explore_Link"]', '[aria-label="Timeline: Trending now"]', '[data-testid="sidebarColumn"] section[aria-labelledby]'],
      search: (q) => `https://x.com/search?q=${q}&src=typed_query`,
    },
    'twitter.com': null, // same as x.com, filled below
    'reddit.com': {
      name: 'Reddit', feed: [/^\/$/, /^\/(best|hot|new|top|rising|news)\/?$/, /^\/posts\/\d{4}\//, /^\/r\/(popular|all)\/?/, /^\/explore\/?/],
      main: ['shreddit-feed', 'main#main-content', 'main', '[role="main"]'],
      hide: ['a[href^="/r/popular"]', 'a[href^="/r/all"]', 'a[href^="/explore"]', 'a[href="/news/"]', 'a[href*="reddit.com/posts/"]', 'faceplate-tracker[noun="popular"]', 'faceplate-tracker[noun="explore"]'],
      search: (q) => `https://www.reddit.com/search/?q=${q}`,
    },
    'threads.net': {
      name: 'Threads', feed: [/^\/$/, /^\/for_you/, /^\/following\/?$/],
      main: ['main', '[role="main"]'], hide: [],
      search: (q) => `https://www.threads.net/search?q=${q}`,
    },
    'threads.com': null,
    'pinterest.com': {
      name: 'Pinterest', feed: [/^\/$/, /^\/today\/?/, /^\/ideas\/?/, /^\/homefeed/],
      main: ['[data-test-id="homefeed-feed"]', '[role="main"]', 'main'],
      hide: ['a[href="/today/"]', 'a[href^="/ideas"]'],
      search: (q) => `https://www.pinterest.com/search/pins/?q=${q}`,
    },
    'tumblr.com': {
      name: 'Tumblr', feed: [/^\/dashboard/, /^\/explore/, /^\/$/],
      main: ['main', '[role="main"]'], hide: ['a[href^="/explore"]'],
      search: (q) => `https://www.tumblr.com/search/${q}`,
    },
    'vk.com': {
      name: 'VK', feed: [/^\/feed\/?/, /^\/clips/, /^\/discover/, /^\/video\/?$/],
      main: ['#page_body', '#content', 'main'], hide: ['#l_nwsf', '#l_clips', 'a[href="/feed"]', 'a[href^="/clips"]'],
      search: (q) => `https://vk.com/search?c%5Bq%5D=${q}&c%5Bsection%5D=auto`,
    },
    'ok.ru': {
      name: 'Odnoklassniki', feed: [/^\/$/, /^\/feed\/?/, /^\/video\/?$/, /^\/video\/(top|new|liveApp)/],
      main: ['#hook_Block_MainFeedsWrapper', '#middleColumn', 'main'], hide: [],
      search: (q) => `https://ok.ru/search?st.query=${q}`,
    },
    'snapchat.com': {
      name: 'Snapchat', feed: [/^\/spotlight/, /^\/discover/, /^\/explore\/?$/],
      main: ['main', '[role="main"]'], hide: ['a[href^="/spotlight"]'], search: null,
    },
    'twitch.tv': {
      name: 'Twitch', feed: [/^\/$/, /^\/directory\/?$/, /^\/directory\/(following|all)/],
      main: ['main', '[role="main"]'], hide: [], search: (q) => `https://www.twitch.tv/search?term=${q}`,
    },
    'kick.com': { name: 'Kick', feed: [/^\/$/, /^\/browse/], main: ['main', '[role="main"]'], hide: [], search: null },
    'bsky.app': {
      name: 'Bluesky', feed: [/^\/$/, /^\/feeds\/?$/, /^\/profile\/[^/]+\/feed\//],
      main: ['main', '[role="main"]'], hide: ['a[href^="/profile/"][href*="/feed/"]'], search: (q) => `https://bsky.app/search?q=${q}`,
    },
  };
  SITES['twitter.com'] = { ...SITES['x.com'], name: 'X' };
  SITES['threads.com'] = { ...SITES['threads.net'], search: (q) => `https://www.threads.com/search?q=${q}` };

  const host = location.hostname.toLowerCase().replace(/^www\.|^m\.|^mobile\./, '');
  const key = Object.keys(SITES).find((d) => host === d || host.endsWith(`.${d}`));
  const site = key && SITES[key];
  if (!site) return; // a listed site without a known layout (e.g. Discord): nothing to hide

  let on = true, strings = null, panel = null, lastUrl = '';
  const EN = {
    socHeading: 'Feed-free mode', socBody: 'The endless feed is switched off here. Search, profiles and messages still work.',
    socSearchLabel: 'Search {site}', socSearchPlaceholder: 'What are you looking for?', socSearchBtn: 'Search',
    socNoSearch: "Use the site's own search or menu to find what you came for.",
  };
  const t = (k, vars = {}) => String((strings && strings[k]) || EN[k] || k).replace(/\{(\w+)\}/g, (m, x) => (vars[x] != null ? String(vars[x]) : m));

  const MAIN = site.main.join(', ');
  const css = document.createElement('style');
  css.id = 'kg-ff-css';
  css.textContent = `
html.kg-ff-on.kg-ff-feed:not(:has(input[type="password"])) :is(${MAIN}):not(:has(#${HOST_ID})) { display: none !important; }
${site.hide.length ? `html.kg-ff-on :is(${site.hide.join(', ')}) { display: none !important; }` : ''}
html:not(.kg-ff-feed) #${HOST_ID}, html:not(.kg-ff-on) #${HOST_ID}, html:has(input[type="password"]) #${HOST_ID} { display: none !important; }
#${HOST_ID} { display: block; width: 100%; }`;
  (document.head || de).appendChild(css);

  const isFeed = (p) => !(site.keep || []).some((r) => r.test(p)) && site.feed.some((r) => r.test(p));

  const PANEL_CSS = `
:host { all: initial; display: block; font-size: medium; }
.p { --surface: #ffffff; --text: #1d1d2c; --text2: #4a4a68; --line: #d9d8e6; --soft: #e9d9ff; --focus: #3b3bb3; --accent: #282858; --accent-text: #ffffff;
  box-sizing: border-box; font-family: system-ui, -apple-system, "Segoe UI", Roboto, "Noto Sans", "Noto Sans Georgian", "BPG Arial", sans-serif;
  font-size: max(1em, 16px); line-height: 1.5; color: var(--text); background: var(--surface); border: 1px solid var(--line); border-radius: 18px;
  max-width: 40em; margin: 24px auto; padding: clamp(16px, 4vw, 28px); animation: kg-rise 220ms cubic-bezier(.2,.7,.2,1) both; }
@media (prefers-color-scheme: dark) { .p { --surface: #212142; --text: #f2f1fa; --text2: #c3c2dc; --line: #3a3a63; --soft: #2e2a4f; --focus: #9fa0ff; --accent: #fedb71; --accent-text: #15152b; } }
*, *::before, *::after { box-sizing: border-box; }
h2 { font-size: 1.25em; line-height: 1.25; margin: 0 0 .4em; font-weight: 700; }
p { margin: 0 0 .9em; color: var(--text2); }
form { display: flex; flex-wrap: wrap; gap: 10px; }
label { display: block; width: 100%; font-weight: 650; }
input { flex: 1 1 12em; min-width: 0; min-height: 3em; padding: .5em 1em; font: inherit; color: var(--text); background: var(--surface); border: 2px solid var(--line); border-radius: 999px; }
input:focus { border-color: var(--focus); outline: none; }
input:focus-visible, button:focus-visible { outline: 3px solid var(--focus); outline-offset: 2px; }
button { min-height: 3em; min-width: 44px; padding: .5em 1.3em; font: inherit; font-weight: 650; border: 0; border-radius: 999px; background: var(--accent); color: var(--accent-text); cursor: pointer; }
@keyframes kg-rise { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: none; } }
@keyframes kg-fade { from { opacity: 0; } to { opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .p { animation-name: kg-fade; } }`;

  const el = (tag, attrs = {}, text) => {
    const e = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
    if (text != null) e.textContent = text;
    return e;
  };

  function buildPanel() {
    panel = el('div', { id: HOST_ID });
    const root = panel.attachShadow({ mode: 'open' });
    for (const ev of ['keydown', 'keypress', 'keyup']) root.addEventListener(ev, (e) => e.stopPropagation());
    panel._fill = () => {
      const s = el('style'); s.textContent = PANEL_CSS;
      const box = el('section', { class: 'p', 'aria-labelledby': 'kgffh' });
      box.append(el('h2', { id: 'kgffh' }, t('socHeading')), el('p', {}, t('socBody')));
      if (site.search) {
        const f = el('form', { role: 'search' });
        const input = el('input', { id: 'kgffq', type: 'search', autocomplete: 'off', placeholder: t('socSearchPlaceholder') });
        f.append(el('label', { for: 'kgffq' }, t('socSearchLabel', { site: site.name })), input, el('button', { type: 'submit' }, t('socSearchBtn')));
        f.addEventListener('submit', (e) => {
          e.preventDefault();
          const q = input.value.trim();
          if (q) location.assign(site.search(encodeURIComponent(q))); else input.focus();
        });
        box.append(f);
      } else box.append(el('p', {}, t('socNoSearch')));
      root.replaceChildren(s, box);
    };
    panel._fill();
  }

  function place() {
    const main = document.querySelector(MAIN);
    if (!panel) buildPanel();
    if (main && main.parentElement) {
      if (panel.nextElementSibling !== main) main.before(panel);
    } else if (document.body && !panel.isConnected) {
      document.body.prepend(panel);
    }
  }

  function route() {
    const feed = on && isFeed(location.pathname);
    de.classList.toggle('kg-ff-on', on);
    de.classList.toggle('kg-ff-feed', feed);
    if (feed) place();
  }

  let pending = false;
  const schedule = () => { if (pending) return; pending = true; setTimeout(() => { pending = false; try { route(); } catch { /* never break the site */ } }, 250); };
  setInterval(() => { if (location.href !== lastUrl) { lastUrl = location.href; try { route(); } catch { /* ignore */ } } }, 300);
  window.addEventListener('popstate', schedule);
  new MutationObserver(schedule).observe(de, { childList: true, subtree: true });

  const apply = (s) => {
    if (s && typeof s === 'object') {
      on = s.focusMode !== false;
      if (!strings || s.lang !== apply.lang) {
        apply.lang = s.lang;
        api.runtime.sendMessage({ type: 'kg:focus:i18n', lang: s.lang || 'en' }).then((r) => { if (r && r.strings) { strings = r.strings; if (panel) panel._fill(); } }).catch(() => {});
      }
    }
    route();
  };
  api.storage.local.get('kg.b.focus').then((r) => apply(r && r['kg.b.focus'])).catch(() => {});
  api.storage.onChanged.addListener((c, area) => { if (area === 'local' && c['kg.b.focus']) apply(c['kg.b.focus'].newValue); });
  route();
})();
