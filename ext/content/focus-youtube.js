// Focus mode on YouTube. Runs at document_start on youtube.com and m.youtube.com and follows
// YouTube's single-page navigation. With focus mode on:
//  - the home page (and trending/explore/gaming) shows only a large search box and the curated videos;
//  - Shorts are gone everywhere (shelves, tabs, nav entries); a /shorts/<id> address opens as a normal
//    watch page, or the block page for the 3-5 band;
//  - the watch page has no related-videos column, no end-screen suggestions, autoplay off, and under 13 no comments;
//  - band youtubeMode "curated" (3-5): search results show only the curated channels, with one plain line saying so.
// Hiding is CSS first (injected now, before YouTube paints), then our panel. Our notes sit in the page flow,
// under the thing they belong to; nothing is drawn on top of YouTube's text.
// P2-161: where our search box shows, YouTube's own search (masthead box, voice button, suggestions, the
// narrow-window search button) is hidden and "/" focuses ours; our panel never sits under the fixed masthead.
// The topic filter (content/topics.js, switch in kg.topics) checks our searches, result titles and watch titles.
// Settings come from storage.local kg.b.focus (written by background/videos.js); strings and videos by message.
(() => {
  'use strict';
  // ---- pure part (unit-tested in node through globalThis.__kgFocusYt when there is no document) ----
  // Page kinds: 'feed' = endless grid replaced by our panel (search box + curated videos);
  // 'chome' = channel home: header and tabs stay, the home sections are hidden, our search box sits under the tabs;
  // 'above' = lists the child chose (history, library, playlists): kept, with our search box above them.
  const FEED_PATHS = [/^\/$/, /^\/feed\/(trending|explore|subscriptions)\/?$/, /^\/gaming\/?$/];
  const CHANNEL_HOME = /^\/(?:@[^/]+|channel\/[\w-]+|c\/[^/]+|user\/[^/]+)(?:\/featured)?\/?$/;
  const ABOVE_PATHS = [/^\/feed\/(history|library|you|playlists)\/?$/, /^\/playlist\/?$/];
  function pageKind(path) {
    const p = String(path || '/');
    if (FEED_PATHS.some((r) => r.test(p))) return 'feed';
    if (CHANNEL_HOME.test(p)) return 'chome';
    if (ABOVE_PATHS.some((r) => r.test(p))) return 'above';
    return null;
  }
  const channelKey = (href) => {
    const m = /^(?:https?:\/\/[^/]+)?\/(?:@([^/?#]+)|channel\/([\w-]+))/.exec(href || '');
    if (!m) return null;
    try { return decodeURIComponent(m[1] || m[2]).toLowerCase(); } catch { return null; }
  };
  /**
   * 3-5 watch page: is this video's channel curated? 'ok', 'block' or 'wait'.
   * dataCh = channel id from YouTube's player data for this video id (MAIN world, data-kg-vc), or null;
   * domKeys = channel handles/ids from the owner link, only when the page shows this video id.
   * Unknown after waitMs: 'block' (fail closed for the youngest).
   */
  function curatedVerdict({ curated, dataCh, domKeys = [], elapsedMs = 0, waitMs = 10000 }) {
    const keys = [dataCh, ...domKeys].filter(Boolean).map((k) => String(k).toLowerCase());
    if (keys.some((k) => curated.has(k))) return 'ok';
    if (dataCh) return 'block';
    if (domKeys.length && elapsedMs > 1500) return 'block';
    return elapsedMs > waitMs ? 'block' : 'wait';
  }
  if (typeof document === 'undefined') { globalThis.__kgFocusYt = { pageKind, channelKey, curatedVerdict }; return; }

  const de = document.documentElement;
  if (!de || de.hasAttribute('data-kg-yt')) return;
  de.setAttribute('data-kg-yt', '');
  const api = globalThis.browser ?? globalThis.chrome;
  const MOBILE = /^m\./.test(location.hostname);
  const HOST_ID = 'kg-yt-home', NOTE_ID = 'kg-yt-note', COM_ID = 'kg-yt-com';
  const ID_RE = /^[\w-]{11}$/;
  const EN = {
    ytSearchLabel: 'Search YouTube', ytSearchPlaceholder: 'Search for a video', ytSearchBtn: 'Search',
    ytCuratedNote: 'Search shows videos only from the channels picked for your age.', ytCuratedNone: 'No videos from the picked channels match this search. Try another word.',
    ytVideosTitle: 'Videos for you', ytVideosFrom: 'From channels picked for children of your age.', ytCommentsOff: 'Comments are switched off.',
    gridLabel: 'Videos picked for you', cardLabel: '{title}, from {channel}', gridEmpty: 'New videos are on their way. Please look again a little later.',
    gridOffline: "The videos need the internet. They'll come back when you're online.", gridSaved: 'Showing saved videos.',
  };

  // Optimistic until storage answers: focus on, so the feed never flashes.
  let st = { focusMode: true, ageBand: null, youtubeMode: 'focus', kids: true, lang: 'en' };
  let known = false, strings = null, curated = null, lastUrl = '', vidsFor = '';
  const t = (k, vars = {}) => String((strings && strings[k]) || EN[k] || k).replace(/\{(\w+)\}/g, (m, x) => (vars[x] != null ? String(vars[x]) : m));
  const ask = (msg) => api.runtime.sendMessage(msg).catch(() => null);

  // ---- topic filter (P2-161): switch and words from storage.local kg.topics (written by the background) ----
  let topics = null, topicsAllowYt = false;
  const loadTopics = (v) => {
    try {
      topics = v && v.on && v.data && globalThis.KGTopics ? globalThis.KGTopics.create(v.data) : null;
      topicsAllowYt = !!(v && Array.isArray(v.allow) && v.allow.some((d) => d === 'youtube.com' || /\.youtube\.com$/.test(String(d))));
    } catch { topics = null; }
  };
  const topicHit = (text) => !!topics && !topicsAllowYt && !!text && topics.matchTitle(text).hit;
  const toTopicPage = () => location.replace(api.runtime.getURL('pages/blocked.html?reason=topic&site=youtube.com'));
  const TOPIC_ITEMS = 'ytd-video-renderer, yt-lockup-view-model, ytd-rich-item-renderer, ytd-compact-video-renderer, ytd-grid-video-renderer, ytd-playlist-renderer, ytd-radio-renderer, ytd-channel-renderer, ytd-reel-item-renderer, ytm-video-with-context-renderer, ytm-compact-video-renderer, ytm-rich-item-renderer';
  const TITLE_SEL = '#video-title, a#video-title-link, h3, #channel-title, .yt-lockup-metadata-view-model__title, .yt-lockup-metadata-view-model-wiz__title, .media-item-headline, .compact-media-item-headline';
  /** Hide videos, playlists and channels whose title (and channel name) is on the topic list. */
  function sweepTopics() {
    if (!topics || topicsAllowYt) { for (const e of document.querySelectorAll('[data-kg-topic]')) e.removeAttribute('data-kg-topic'); return; }
    for (const item of document.querySelectorAll(TOPIC_ITEMS)) {
      const ti = item.querySelector(TITLE_SEL);
      const text = [ti && (ti.getAttribute('title') || ti.textContent), ...[...item.querySelectorAll('ytd-channel-name a, .ytd-channel-name a, a[href^="/@"]')].slice(0, 2).map((a) => a.textContent)].filter(Boolean).join(' ');
      const key = text.slice(0, 300);
      if (item.__kgTopicKey === key) continue;
      item.__kgTopicKey = key;
      if (topicHit(text)) item.setAttribute('data-kg-topic', ''); else item.removeAttribute('data-kg-topic');
    }
  }
  /** Watch and results pages: the video's title, or the search words, on the topic list: the topic page. */
  function topicPage() {
    if (!topics || topicsAllowYt) return;
    const p = location.pathname;
    if (/^\/results/.test(p)) {
      const q = new URLSearchParams(location.search).get('search_query') || '';
      if (topicHit(q)) toTopicPage();
      return;
    }
    // Hashtag, channel and playlist pages: the page's own title and header (in-page navigation never reloads).
    if (/^\/(?:hashtag\/|@|channel\/|c\/|user\/|playlist)/.test(p)) {
      let tag = '';
      try { tag = /^\/hashtag\/([^/?#]+)/.test(p) ? decodeURIComponent(p.split('/')[2]) : ''; } catch { /* keep '' */ }
      const head = [...document.querySelectorAll('ytd-hashtag-header-renderer h1, yt-page-header-renderer h1, ytd-c4-tabbed-header-renderer #channel-name, ytd-playlist-header-renderer h1, .page-header-view-model-wiz__page-header-title, ytm-playlist-header-renderer h1')].map((e) => e.textContent).join(' · ');
      if (topicHit(tag) || topicHit(head) || topicHit(document.title.replace(/ - YouTube$/, ''))) toTopicPage();
      return;
    }
    if (!/^\/(watch|shorts\/|live\/)/.test(p)) return;
    const vid = new URLSearchParams(location.search).get('v');
    const flexy = document.querySelector('ytd-watch-flexy');
    const h1 = flexy && (!vid || flexy.getAttribute('video-id') === vid) ? document.querySelector('ytd-watch-metadata h1, ytd-watch-flexy #title h1') : null;
    const title = (h1 && h1.textContent.trim()) || (document.querySelector('ytm-slim-video-metadata-section-renderer h2, .slim-video-information-title') || {}).textContent || '';
    if (topicHit(title) || topicHit(document.title.replace(/ - YouTube$/, ''))) toTopicPage();
  }

  // ---- 1. CSS, now ----
  const SHORTS_LINK = 'a[href^="/shorts/"]';
  const CSS = `
html.kg-hold body { visibility: hidden !important; }
html:not(.kg-feed):not(.kg-chome):not(.kg-above) #${HOST_ID} { display: none !important; }
html.kg-f.kg-ours ytd-masthead #center, html.kg-f.kg-ours ytd-masthead yt-searchbox, html.kg-f.kg-ours ytd-masthead ytd-searchbox,
html.kg-f.kg-ours ytd-masthead #voice-search-button { visibility: hidden !important; pointer-events: none !important; }
html.kg-f.kg-ours ytd-masthead #search-button-narrow, html.kg-f.kg-ours ytd-masthead #voice-search-button,
html.kg-f.kg-ours .ytSearchboxComponentSuggestionsContainer, html.kg-f.kg-ours .sbdd_a, html.kg-f.kg-ours .gstl_50,
html.kg-f.kg-ours ytm-mobile-topbar-renderer ytm-searchbox, html.kg-f.kg-ours ytm-mobile-topbar-renderer yt-searchbox,
html.kg-f.kg-ours ytm-mobile-topbar-renderer .topbar-button-search-button, html.kg-f.kg-ours ytm-mobile-topbar-renderer button.topbar-menu-button-search,
html.kg-f.kg-feed #frosted-glass { display: none !important; }
[data-kg-topic] { display: none !important; }
html.kg-f.kg-chome ytd-page-manager > ytd-browse:not([hidden]) > ytd-two-column-browse-results-renderer { display: none !important; }
#${HOST_ID} { display: block; width: 100%; }
html.kg-f.kg-feed ytd-page-manager > ytd-browse:not([hidden]) > :not(#${HOST_ID}),
html.kg-f.kg-feed ytm-browse > :not(#${HOST_ID}),
html.kg-f.kg-feed ytm-rich-grid-renderer,
html.kg-f.kg-feed ytm-feed-filter-chip-bar-renderer,
html.kg-f.kg-feed ytd-feed-filter-chip-bar-renderer { display: none !important; }
html.kg-f ytd-reel-shelf-renderer, html.kg-f ytm-reel-shelf-renderer,
html.kg-f ytd-rich-shelf-renderer[is-shorts], html.kg-f ytd-rich-section-renderer:has(ytd-rich-shelf-renderer[is-shorts]),
html.kg-f ytd-rich-section-renderer:has(ytm-shorts-lockup-view-model, ytm-shorts-lockup-view-model-v2),
html.kg-f ytm-shorts-lockup-view-model, html.kg-f ytm-shorts-lockup-view-model-v2,
html.kg-f grid-shelf-view-model:has(ytm-shorts-lockup-view-model, ytm-shorts-lockup-view-model-v2, ${SHORTS_LINK}),
html.kg-f ytd-guide-entry-renderer:has(a[title="Shorts"], a[href^="/shorts"]),
html.kg-f ytd-mini-guide-entry-renderer:has(a[title="Shorts"], a[href^="/shorts"]), html.kg-f ytd-mini-guide-entry-renderer[aria-label="Shorts"],
html.kg-f ytm-pivot-bar-item-renderer:has(.pivot-shorts, [class*="pivot-shorts"]),
html.kg-f ytd-video-renderer:has(${SHORTS_LINK}), html.kg-f ytd-rich-item-renderer:has(${SHORTS_LINK}),
html.kg-f ytd-grid-video-renderer:has(${SHORTS_LINK}), html.kg-f ytd-compact-video-renderer:has(${SHORTS_LINK}),
html.kg-f yt-lockup-view-model:has(${SHORTS_LINK}), html.kg-f ytm-video-with-context-renderer:has(${SHORTS_LINK}),
html.kg-f yt-tab-shape[tab-title="Shorts"], html.kg-f [data-kg-shorts] { display: none !important; }
html.kg-f.kg-watch #related, html.kg-f.kg-watch ytd-watch-next-secondary-results-renderer,
html.kg-f.kg-watch ytm-item-section-renderer[section-identifier="related-items"],
html.kg-f .html5-endscreen, html.kg-f .ytp-endscreen-content, html.kg-f .ytp-ce-element, html.kg-f .ytp-endscreen-previous, html.kg-f .ytp-endscreen-next,
html.kg-f .ytp-autonav-endscreen, html.kg-f .ytp-autonav-endscreen-countdown-overlay, html.kg-f .ytp-autonav-endscreen-upnext-container,
html.kg-f .ytp-videowall-still, html.kg-f .ytp-pause-overlay, html.kg-f .ytp-suggestion-set, html.kg-f .ytp-fullscreen-grid,
html.kg-f .ytp-fullscreen-grid-peeking, html.kg-f .ytp-next-button, html.kg-f .ytp-autonav-toggle-button-container,
html.kg-f .ytp-button[data-tooltip-target-id="ytp-autonav-toggle-button"], html.kg-f .ytm-autonav-toggle-button-container { display: none !important; }
html.kg-f.kg-nocom ytd-comments#comments, html.kg-f.kg-nocom #comment-teaser, html.kg-f.kg-nocom ytd-comments-entry-point-header-renderer,
html.kg-f.kg-nocom ytd-engagement-panel-section-list-renderer[target-id="engagement-panel-comments-section"],
html.kg-f.kg-nocom ytd-item-section-renderer[section-identifier="comment-item-section"],
html.kg-f.kg-nocom ytm-comments-entry-point-header-renderer, html.kg-f.kg-nocom ytm-comment-section-renderer,
html.kg-f.kg-nocom #chat-container, html.kg-f.kg-nocom ytd-live-chat-frame { display: none !important; }
html.kg-f.kg-cur ytd-search ytd-item-section-renderer #contents > :not(ytd-video-renderer[data-kg-ok]):not(yt-lockup-view-model[data-kg-ok]),
html.kg-f.kg-cur ytd-search ytd-section-list-renderer > #contents > :not(ytd-item-section-renderer):not(#${NOTE_ID}),
html.kg-f.kg-cur ytd-search ytd-item-section-renderer:not(:has(ytd-video-renderer[data-kg-ok], yt-lockup-view-model[data-kg-ok])) ytd-continuation-item-renderer,
html.kg-f.kg-cur ytm-search ytm-item-section-renderer > :not([data-kg-ok]) { display: none !important; }
html:not(.kg-f) #${NOTE_ID}, html:not(.kg-cur) #${NOTE_ID}, html:not(.kg-nocom) #${COM_ID}, html:not(.kg-f) #${COM_ID} { display: none !important; }
#${NOTE_ID}, #${COM_ID} { display: block; margin: 12px 0 16px; padding: 10px 14px; border-radius: 12px; font: 500 medium/1.5 system-ui, -apple-system, "Segoe UI", Roboto, "Noto Sans", "Noto Sans Georgian", sans-serif;
  background: #e9d9ff; color: #1d1d2c; max-width: 52rem; }
html[dark] #${NOTE_ID}, html[dark] #${COM_ID} { background: #2e2a4f; color: #f2f1fa; }
`;
  const style = document.createElement('style');
  style.id = 'kg-yt-css';
  style.textContent = CSS;
  (document.head || de).appendChild(style);

    const setClass = (c, on) => de.classList.toggle(c, !!on);

  // ---- 2. Shorts addresses ----
  function shortsTarget(u) {
    const m = /^\/shorts\/([\w-]{11})/.exec(u.pathname);
    if (m) return { kind: 'video', id: m[1], url: `/watch?v=${m[1]}` };
    const c = /^(\/(?:@[^/]+|channel\/[\w-]+|c\/[^/]+|user\/[^/]+))\/shorts\/?$/.exec(u.pathname);
    if (c) return { kind: 'tab', url: `${c[1]}/videos` };
    return null;
  }
  function leaveShorts(target) {
    if (target.kind === 'video' && st.youtubeMode === 'curated') {
      location.replace(api.runtime.getURL('pages/blocked.html?reason=shorts'));
    } else location.replace(target.url);
  }

  // ---- 3. the home panel (shadow DOM, so YouTube's styles do not touch it and ours do not leak) ----
  const PANEL_CSS = `
:host { all: initial; display: block; font-size: medium; }
.p { --navy: #282858; --surface: #ffffff; --text: #1d1d2c; --text2: #4a4a68; --line: #d9d8e6; --soft: #e9d9ff; --focus: #3b3bb3; --accent: #282858; --accent-text: #ffffff; --bg: transparent;
  box-sizing: border-box; font-family: system-ui, -apple-system, "Segoe UI", Roboto, "Noto Sans", "Noto Sans Georgian", "BPG Arial", sans-serif;
  font-size: max(1em, 16px); line-height: 1.5; color: var(--text); max-width: 76em; margin: 0 auto; padding: 32px 16px 48px; animation: kg-rise 220ms cubic-bezier(.2,.7,.2,1) both; }
:host(.dark) .p { --surface: #212142; --text: #f2f1fa; --text2: #c3c2dc; --line: #3a3a63; --soft: #2e2a4f; --focus: #9fa0ff; --accent: #fedb71; --accent-text: #15152b; }
*, *::before, *::after { box-sizing: border-box; }
h1 { font-size: clamp(1.5em, 1.2em + 1.4vw, 2.25em); line-height: 1.25; margin: 0 0 .6em; font-weight: 750; text-align: center; text-wrap: balance; }
h2 { font-size: 1.25em; line-height: 1.25; margin: 1.6em 0 .25em; font-weight: 700; }
p { margin: 0 0 .75em; }
form { display: flex; flex-wrap: wrap; gap: 10px; max-width: 46em; margin: 0 auto; }
label.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
input { flex: 1 1 12em; min-width: 0; min-height: 3.25em; padding: .5em 1.1em; font: inherit; font-size: 1.125em; color: var(--text); background: var(--surface);
  border: 2px solid var(--line); border-radius: 999px; }
input:focus { border-color: var(--focus); outline: none; }
input:focus-visible, button:focus-visible, a:focus-visible { outline: 3px solid var(--focus); outline-offset: 2px; }
button { min-height: 3.25em; min-width: 44px; padding: .55em 1.4em; font: inherit; font-weight: 650; border: 2px solid transparent; border-radius: 999px;
  background: var(--accent); color: var(--accent-text); cursor: pointer; max-width: 100%; white-space: normal; }
.saved { margin: 12px 0 0; color: var(--text2); font-size: .9375em; }
.note { max-width: 46em; margin: 12px auto 0; padding: 10px 14px; border-radius: 12px; background: var(--soft); text-align: center; }
.from { color: var(--text2); font-size: .9375em; }
ul { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 13.5em), 1fr)); gap: 16px; list-style: none; margin: 16px 0 0; padding: 0; }
li { min-width: 0; }
a.card { display: flex; flex-direction: column; gap: 6px; height: 100%; min-height: 44px; padding: 0 0 10px; border-radius: 12px; overflow: hidden;
  color: var(--text); text-decoration: none; background: var(--surface); border: 1px solid var(--line); transition: transform 220ms cubic-bezier(.2,.7,.2,1), border-color 220ms; }
a.card:hover { transform: translateY(-2px); border-color: var(--accent); }
img { display: block; width: 100%; height: auto; aspect-ratio: 16 / 9; object-fit: cover; background: var(--soft); }
.ti { padding: 0 12px; font-weight: 650; line-height: 1.35; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
.ch { padding: 0 12px; font-size: .9375em; color: var(--text2); overflow-wrap: anywhere; }
.p.compact { padding: 16px 16px 8px; animation: none; }
.p.compact form { margin: 0 auto; }
.state { margin: 16px 0 0; padding: 14px 16px; border-radius: 12px; background: var(--soft); }
@keyframes kg-rise { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: none; } }
@keyframes kg-fade { from { opacity: 0; } to { opacity: 1; } }
@media (prefers-reduced-motion: reduce) {
  .p { animation-name: kg-fade; }
  a.card { transition: opacity 220ms cubic-bezier(.2,.7,.2,1), border-color 220ms; }
  a.card:hover { transform: none; opacity: .92; }
}`;

  let host = null, shadow = null, panelMode = 'feed';
  const h = (tag, attrs = {}, text) => {
    const e = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
    if (text != null) e.textContent = text;
    return e;
  };

  function buildPanel() {
    host = document.createElement('div');
    host.id = HOST_ID;
    shadow = host.attachShadow({ mode: 'open' });
    // Keys typed in our box must not reach YouTube's keyboard shortcuts.
    for (const ev of ['keydown', 'keypress', 'keyup']) shadow.addEventListener(ev, (e) => e.stopPropagation());
    fillPanel();
  }

  function fillPanel() {
    if (!shadow) return;
    const full = panelMode === 'feed';
    host.dataset.mode = panelMode;
    const css = h('style'); css.textContent = PANEL_CSS;
    // No heading above the search box (the owner, 4 Nov, P2-161 follow-up): just the box.
    const wrap = h('section', { class: full ? 'p' : 'p compact', 'aria-label': t('ytSearchLabel') });
    const form = h('form', { role: 'search', action: '/results', method: 'get' });
    const input = h('input', { id: 'kgq', name: 'search_query', type: 'search', autocomplete: 'off', placeholder: t('ytSearchPlaceholder'), 'aria-label': t('ytSearchLabel') });
    form.append(h('label', { class: 'sr', for: 'kgq' }, t('ytSearchLabel')), input, h('button', { type: 'submit' }, t('ytSearchBtn')));
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const q = input.value.trim();
      if (!q) { input.focus(); return; }
      if (topicHit(q)) { toTopicPage(); return; }
      location.assign(`/results?search_query=${encodeURIComponent(q)}`);
    });
    wrap.append(form);
    if (st.youtubeMode === 'curated') wrap.append(h('p', { class: 'note' }, t('ytCuratedNote')));
    if (!full) { shadow.replaceChildren(css, wrap); vidsFor = ''; return; }
    wrap.append(h('h2', { id: 'kgv' }, t('ytVideosTitle')), h('p', { class: 'from' }, t('ytVideosFrom')));
    const grid = h('div', { id: 'kggrid', 'aria-busy': 'true' });
    wrap.append(grid);
    shadow.replaceChildren(css, wrap);
    vidsFor = '';
    loadVideos();
  }

  async function loadVideos() {
    const key = `${st.ageBand}|${st.lang}`;
    if (!shadow || vidsFor === key) return;
    vidsFor = key;
    const grid = shadow.getElementById('kggrid');
    if (!grid) { vidsFor = ''; return; }
    const res = await ask({ type: 'kg:focus:videos', ageBand: st.ageBand || undefined, lang: st.lang, limit: 24 });
    if (!grid.isConnected) return;
    grid.removeAttribute('aria-busy');
    const vids = (res && Array.isArray(res.videos) ? res.videos : []).filter((v) => v && ID_RE.test(v.id) && !topicHit(`${v.title} ${v.channel || ''}`));
    if (!vids.length) {
      // The offline line only when the browser says it is offline (P2-161); feeds that do not answer are not "offline".
      grid.replaceChildren(h('p', { class: 'state' }, t(navigator.onLine === false ? 'gridOffline' : 'gridEmpty')));
      if (!res) vidsFor = ''; // background not ready: try again on the next pass
      return;
    }
    const ul = h('ul', { 'aria-labelledby': 'kgv' });
    for (const v of vids) {
      const a = h('a', { class: 'card', href: `/watch?v=${v.id}`, 'aria-label': t('cardLabel', { title: v.title, channel: v.channel }) });
      const img = h('img', { src: `https://i.ytimg.com/vi/${v.id}/mqdefault.jpg`, alt: '', loading: 'lazy', decoding: 'async', width: '320', height: '180' });
      img.addEventListener('error', () => img.removeAttribute('src'), { once: true });
      a.append(img, h('span', { class: 'ti' }, v.title), h('span', { class: 'ch' }, v.channel));
      const li = h('li'); li.append(a); ul.append(li);
    }
    if (res && res.saved) grid.replaceChildren(ul, h('p', { class: 'saved' }, t('gridSaved')));
    else grid.replaceChildren(ul);
  }

  // Nothing of ours under YouTube's fixed bars (masthead, chip bar): if they reach lower than the top of our
  // panel (at scroll 0), the panel gets that much room on top.
  let barsRO = null, barsEl = null;
  function clearBars() {
    if (!host || !host.isConnected) return;
    // YouTube sizes its masthead after our panel may be placed: measure again when it changes size.
    const mc = document.getElementById('masthead-container');
    if (mc && mc !== barsEl && typeof ResizeObserver === 'function') { barsEl = mc; (barsRO ||= new ResizeObserver(() => clearBars())).observe(mc); }
    let bottom = 0;
    for (const b of document.querySelectorAll('#masthead-container, ytd-app > #frosted-glass, #frosted-glass, ytm-mobile-topbar-renderer, header.mobile-topbar-header')) {
      const cs = getComputedStyle(b);
      if (cs.display === 'none' || cs.visibility === 'hidden' || (cs.position !== 'fixed' && cs.position !== 'sticky')) continue;
      const r = b.getBoundingClientRect();
      if (r.height > 0 && r.top <= 1) bottom = Math.max(bottom, r.bottom);
    }
    // The host's top in page coordinates (its padding is inside it); fixed bars are measured at the top.
    const want = Math.max(0, Math.ceil(bottom - (host.getBoundingClientRect().top + window.scrollY)));
    const pad = parseFloat(host.style.paddingTop) || 0;
    if (Math.abs(want - pad) >= 1) host.style.setProperty('padding-top', `${want}px`, 'important');
  }

  function placePanel(kind) {
    const target = MOBILE
      ? document.querySelector('ytm-browse') || document.querySelector('#app ytm-rich-grid-renderer')?.parentElement
      : document.querySelector('ytd-page-manager > ytd-browse:not([hidden])');
    if (!target) return;
    const mode = kind === 'feed' ? 'feed' : 'compact';
    if (!host) { panelMode = mode; buildPanel(); } else if (panelMode !== mode) { panelMode = mode; fillPanel(); }
    // Channel home: under the channel header and its tabs. Elsewhere: first in the page.
    // The header element counts before it has height: it gets its height from layout,
    // which fires no DOM change, so waiting for the height could leave our box above it.
    const header = kind === 'chome' && !MOBILE ? [...target.children].find((c) => c.id === 'header') : null;
    if (header) { if (header.nextElementSibling !== host) header.after(host); }
    else if (host.parentElement !== target || target.firstElementChild !== host) target.prepend(host);
    host.classList.toggle('dark', de.hasAttribute('dark') || (MOBILE && matchMedia('(prefers-color-scheme: dark)').matches));
    loadVideos();
    clearBars();
  }

  // ---- 4. curated search (3-5) ----
  async function loadCurated() {
    if (curated) return curated;
    const r = await ask({ type: 'kg:focus:channels', ageBand: st.ageBand || undefined });
    if (!r || !Array.isArray(r.channels)) return null;
    curated = new Set();
    for (const c of r.channels) { curated.add(c.id.toLowerCase()); if (c.handle) curated.add(c.handle.replace(/^@/, '').toLowerCase()); }
    return curated;
  }
  let noneTimer = null;
  function markResults() {
    const list = document.querySelector('ytd-search ytd-section-list-renderer > #contents') || document.querySelector('ytm-search');
    if (!list) return;
    let note = document.getElementById(NOTE_ID);
    if (!note) { note = h('p', { id: NOTE_ID, role: 'note' }); }
    if (list.firstElementChild !== note) list.prepend(note);
    if (!curated) { loadCurated().then(() => markResults()); return; }
    let ok = 0;
    for (const item of document.querySelectorAll('ytd-search ytd-video-renderer, ytd-search yt-lockup-view-model, ytm-search ytm-video-with-context-renderer, ytm-search ytm-compact-video-renderer')) {
      const links = item.querySelectorAll('a[href^="/@"], a[href^="/channel/"]');
      let good = false;
      for (const a of links) { const k = channelKey(a.getAttribute('href')); if (k && curated.has(k)) { good = true; break; } }
      if (good && !item.querySelector(SHORTS_LINK)) { item.setAttribute('data-kg-ok', ''); ok++; } else item.removeAttribute('data-kg-ok');
    }
    note.textContent = t('ytCuratedNote');
    clearTimeout(noneTimer);
    if (!ok) noneTimer = setTimeout(() => { if (de.classList.contains('kg-cur') && !document.querySelector('[data-kg-ok]')) note.textContent = `${t('ytCuratedNote')} ${t('ytCuratedNone')}`; }, 2500);
  }

  // ---- 5. watch page ----
  function watchPage() {
    const b = document.querySelector('.ytp-autonav-toggle-button[aria-checked="true"]');
    if (b) b.click();
    const mb = document.querySelector('.ytm-autonav-toggle-button-container [aria-pressed="true"]');
    if (mb) mb.click();
    if (st.kids) {
      const com = document.querySelector('ytd-watch-flexy ytd-comments#comments') || document.querySelector('ytm-comments-entry-point-header-renderer');
      if (com && com.parentElement) {
        let n = document.getElementById(COM_ID);
        if (!n) n = h('p', { id: COM_ID }, t('ytCommentsOff'));
        if (com.previousElementSibling !== n) com.before(n);
      }
    }
  }

  // 3-5: a watch page of a channel that is not curated goes to the block page. The player is hidden and
  // paused while we check (YouTube's data for this video, or the owner link once the page shows this video).
  let watchCheck = null;
  function dataChannel(vid) {
    try { const m = JSON.parse(de.getAttribute('data-kg-vc') || '{}'); return typeof m[vid] === 'string' ? m[vid] : null; } catch { return null; }
  }
  function domChannelKeys(vid) {
    const flexy = document.querySelector('ytd-watch-flexy');
    if (!flexy || flexy.getAttribute('video-id') !== vid) return [];
    return [...document.querySelectorAll('ytd-watch-metadata ytd-video-owner-renderer a[href], ytd-watch-flexy #owner a[href]')]
      .map((a) => channelKey(a.getAttribute('href'))).filter(Boolean);
  }
  async function curatedWatch() {
    const vid = new URLSearchParams(location.search).get('v');
    if (!vid || !ID_RE.test(vid)) return;
    if (watchCheck && watchCheck.vid === vid && watchCheck.done) return;
    if (!watchCheck || watchCheck.vid !== vid) watchCheck = { vid, start: Date.now(), done: false, paused: new Set() };
    setClass('kg-hold', true);
    for (const v of document.querySelectorAll('video')) { try { if (!v.paused) { v.pause(); watchCheck.paused.add(v); } } catch { /* fine */ } }
    const set = await loadCurated();
    if (!set || !watchCheck || watchCheck.vid !== vid) { setTimeout(schedule, 300); return; }
    const verdict = curatedVerdict({ curated: set, dataCh: dataChannel(vid), domKeys: MOBILE ? [] : domChannelKeys(vid), elapsedMs: Date.now() - watchCheck.start });
    if (verdict === 'ok') {
      watchCheck.done = true; setClass('kg-hold', false);
      for (const v of watchCheck.paused) { try { v.play().catch(() => {}); } catch { /* fine */ } }
    }
    else if (verdict === 'block') { watchCheck.done = true; location.replace(api.runtime.getURL('pages/blocked.html?reason=curated')); }
    else setTimeout(schedule, 300);
  }

  // Text-only Shorts entries (localized nav and chips) that CSS cannot match.
  function sweepShortsText() {
    for (const e of document.querySelectorAll('yt-chip-cloud-chip-renderer, ytd-guide-entry-renderer, ytd-mini-guide-entry-renderer, yt-tab-shape, tp-yt-paper-tab, ytm-pivot-bar-item-renderer, ytm-chip-cloud-chip-renderer')) {
      if (e.hasAttribute('data-kg-shorts')) continue;
      const txt = (e.textContent || '').trim();
      if (/^(Shorts|Шортс|შორტსი)$/i.test(txt)) e.setAttribute('data-kg-shorts', '');
    }
  }

  // ---- 6. routing ----
  function route() {
    try { topicPage(); sweepTopics(); } catch { /* keep YouTube usable */ }
    const on = st.focusMode !== false;
    setClass('kg-f', on);
    de.setAttribute('data-kg-focus', on ? 'on' : 'off');
    const p = location.pathname;
    const kind = on ? pageKind(p) : null;
    const feed = kind === 'feed';
    setClass('kg-feed', feed);
    setClass('kg-chome', kind === 'chome');
    setClass('kg-above', kind === 'above');
    setClass('kg-ours', !!kind);
    setClass('kg-watch', /^\/watch/.test(p));
    setClass('kg-nocom', on && st.kids);
    setClass('kg-cur', on && st.youtubeMode === 'curated' && /^\/results/.test(p));
    if (!on) { setClass('kg-hold', false); return; }
    const sh = shortsTarget(location);
    if (sh) {
      if (known) leaveShorts(sh); else setClass('kg-hold', true);
      return;
    }
    const curatedWatchPage = st.youtubeMode === 'curated' && /^\/watch/.test(p);
    if (!curatedWatchPage) { setClass('kg-hold', false); watchCheck = null; }
    if (kind) placePanel(kind);
    if (de.classList.contains('kg-cur')) markResults();
    if (/^\/watch/.test(p)) watchPage();
    if (curatedWatchPage && known) curatedWatch();
    sweepShortsText();
  }

  let pending = false;
  const schedule = () => { if (pending) return; pending = true; setTimeout(() => { pending = false; try { route(); } catch { /* keep YouTube usable */ } }, 200); };
  const onUrl = () => { if (location.href !== lastUrl) { lastUrl = location.href; try { route(); } catch { /* ignore */ } } };

  // Links to Shorts open as normal videos (or nothing, for 3-5) before YouTube's router sees the click.
  document.addEventListener('click', (e) => {
    if (st.focusMode === false || e.button !== 0) return;
    const a = e.target && e.target.closest && e.target.closest('a[href]');
    if (!a) return;
    let u; try { u = new URL(a.href, location.href); } catch { return; }
    if (!/(^|\.)youtube\.com$/.test(u.hostname)) return;
    const sh = shortsTarget(u);
    if (!sh) return;
    e.preventDefault(); e.stopImmediatePropagation();
    leaveShorts(sh);
  }, true);

  // "/" focuses our search box where it shows (YouTube's own box is hidden there). Capture on window runs
  // before YouTube's shortcut; keys typed inside our box are left alone.
  window.addEventListener('keydown', (e) => {
    if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey || st.focusMode === false || !de.classList.contains('kg-ours') || !shadow) return;
    if (host && e.composedPath().includes(host)) return;
    const tgt = e.target;
    if (tgt && (tgt.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(tgt.tagName || ''))) return;
    const input = shadow.getElementById('kgq');
    if (!input) return;
    e.preventDefault(); e.stopImmediatePropagation();
    input.focus();
  }, true);

  for (const ev of ['yt-navigate-start', 'yt-navigate-finish', 'yt-page-data-updated', 'popstate', 'state-navigateend']) {
    (ev === 'popstate' ? window : document).addEventListener(ev, () => { lastUrl = ''; onUrl(); schedule(); }, true);
  }
  setInterval(onUrl, 400);
  new MutationObserver(schedule).observe(de, { childList: true, subtree: true });

  // ---- 7. settings ----
  const applyState = (s, fromStore) => {
    if (s && typeof s === 'object') {
      const bandChanged = s.ageBand !== st.ageBand;
      const langChanged = s.lang !== st.lang;
      st = { ...st, ...s };
      if (bandChanged) curated = null;
      if (langChanged || !strings) loadStrings();
      else if (bandChanged && shadow) fillPanel();
    }
    if (fromStore) known = true;
    lastUrl = '';
    onUrl();
  };
  async function loadStrings() {
    const r = await ask({ type: 'kg:focus:i18n', lang: st.lang });
    if (r && r.strings) { strings = r.strings; if (shadow) fillPanel(); const n = document.getElementById(COM_ID); if (n) n.textContent = t('ytCommentsOff'); }
  }
  api.storage.local.get('kg.b.focus').then(async (r) => {
    let s = r && r['kg.b.focus'];
    if (!s) s = await ask({ type: 'kg:focus:state' });
    applyState(s, true);
  }).catch(() => applyState(null, true));
  api.storage.onChanged.addListener((c, area) => {
    if (area !== 'local') return;
    if (c['kg.topics']) { loadTopics(c['kg.topics'].newValue); lastUrl = ''; onUrl(); }
    if (c['kg.b.focus']) applyState(c['kg.b.focus'].newValue, true);
  });
  api.storage.local.get('kg.topics').then((r) => { loadTopics(r && r['kg.topics']); lastUrl = ''; onUrl(); }).catch(() => {});
  window.addEventListener('resize', () => { if (host && host.isConnected) clearBars(); });


  onUrl();
})();
