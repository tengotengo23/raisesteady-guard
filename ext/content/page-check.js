// Page check and video heartbeat. Classic content script, all http(s) frames, document_start.
// Light on purpose: no MutationObserver; one check when the DOM is ready and one 3 s later.
(() => {
  'use strict';
  if (globalThis.__kgPageCheck) return;
  globalThis.__kgPageCheck = true;
  const api = globalThis.browser ?? globalThis.chrome;
  const isTop = window === window.top;

  const send = (msg) => {
    try { return api.runtime.sendMessage(msg).catch(() => null); } catch { return Promise.resolve(null); } // extension reloaded
  };

  // ---- page check (top frame only) ----
  const SKIP = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'SVG', 'IFRAME', 'TEXTAREA']);
  function textSample(limit) {
    const body = document.body;
    if (!body) return '';
    const parts = [];
    let len = 0;
    const walker = document.createTreeWalker(body, NodeFilter.SHOW_TEXT, {
      acceptNode: (n) => (n.parentElement && SKIP.has(n.parentElement.tagName) ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
    });
    while (len < limit && walker.nextNode()) {
      const v = walker.currentNode.nodeValue;
      if (v && v.trim()) { parts.push(v); len += v.length + 1; }
    }
    return parts.join(' ').slice(0, limit);
  }
  function collect() {
    const meta = (sel) => { const el = document.querySelector(sel); return el ? el.getAttribute('content') || '' : ''; };
    const metas = [...document.querySelectorAll('meta[content]')].slice(0, 80).map((m) => ({
      name: m.getAttribute('name') || m.getAttribute('http-equiv') || m.getAttribute('property') || '',
      content: m.getAttribute('content') || '',
    }));
    return {
      title: document.title,
      description: meta('meta[name="description" i]') || meta('meta[property="og:description" i]'),
      keywords: meta('meta[name="keywords" i]'),
      headings: [...document.querySelectorAll('h1, h2, h3')].slice(0, 30).map((h) => h.textContent || ''),
      text: textSample(20000),
      url: location.hostname + location.pathname,
      metas,
    };
  }
  // Topic filter (P2-161): the switch and the word list come from storage.local (kg.topics, written by the
  // background). Only the title, og: tags, description, keywords, h1-h2 and the address path are read.
  let topics = null, topicsAllow = [], helplines = [];
  const topicsReady = (async () => {
    try {
      const t = (await api.storage.local.get('kg.topics'))['kg.topics'];
      if (t && Array.isArray(t.helplines)) helplines = t.helplines.map((d) => String(d).toLowerCase());
      if (t && t.on && t.data && globalThis.KGTopics) { topics = globalThis.KGTopics.create(t.data); topicsAllow = Array.isArray(t.allow) ? t.allow : []; }
    } catch { /* storage unavailable: no topic check */ }
  })();
  // Helplines are never flagged, by the adult-content check either (P2-161).
  const isHelpline = (host) => helplines.some((d) => host === d || host.endsWith('.' + d));
  const allowed = (host) => topicsAllow.some((d) => host === d || host.endsWith('.' + d));
  function topicFields() {
    const meta = (sel) => { const el = document.querySelector(sel); return el ? el.getAttribute('content') || '' : ''; };
    return {
      host: location.hostname.replace(/^www\./, ''), path: location.pathname,
      title: document.title, ogTitle: meta('meta[property="og:title" i]'),
      description: meta('meta[name="description" i]'), ogDescription: meta('meta[property="og:description" i]'),
      keywords: meta('meta[name="keywords" i]'),
      headings: [...document.querySelectorAll('h1, h2')].slice(0, 20).map((h) => h.textContent || ''),
    };
  }
  let flagged = false;
  function hide() {
    flagged = true;
    document.documentElement.style.setProperty('visibility', 'hidden', 'important');
    for (const v of document.querySelectorAll('video, audio')) { try { v.pause(); } catch { /* ignore */ } }
  }
  const unhide = () => { flagged = false; document.documentElement.style.removeProperty('visibility'); };
  function check() {
    if (flagged || !globalThis.KGClassify) return;
    if (isHelpline(location.hostname.toLowerCase().replace(/^www\./, ''))) return;
    let res;
    try { res = globalThis.KGClassify.classify(collect()); } catch { return; }
    if (res.block) {
      hide();
      send({ type: 'kg:core:page-flag', reason: 'content', why: res.reason, score: res.score }).then((r) => { if (r && r.ok === false) unhide(); });
      return;
    }
    if (!topics) return;
    const f = topicFields();
    if (allowed(f.host)) return;
    let t;
    try { t = topics.matchPage(f); } catch { return; }
    if (!t.hit) return;
    hide();
    send({ type: 'kg:core:page-flag', reason: 'topic', why: t.why }).then((r) => { if (r && r.ok === false) unhide(); }); // the background said no
  }
  if (isTop && /^https?:$/.test(location.protocol)) {
    const start = () => { topicsReady.then(() => { check(); setTimeout(check, 3000); }); };
    // Single-page sites change the address and title without a reload: look again (P2-161).
    let lastHref = location.href;
    const again = () => { if (flagged || location.href === lastHref) return; lastHref = location.href; setTimeout(check, 800); setTimeout(check, 3000); };
    window.addEventListener('popstate', again);
    try { api.runtime.onMessage.addListener((m) => { if (m && m.type === 'kg:core:recheck') again(); return false; }); } catch { /* no runtime */ }
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start, { once: true });
    else start();
  }

  // ---- video heartbeat (every frame) ----
  const HEARTBEAT_MS = 15000;
  const seen = new Set();
  let lastSent = 0, lastState = false, over = false;
  const audible = (v) => !v.muted && v.volume > 0;
  const visible = (v) => v.offsetWidth >= 2 && v.offsetHeight >= 2 && document.visibilityState === 'visible';
  const playing = (v) => v.isConnected && !v.paused && !v.ended && v.readyState > 2 && (audible(v) || visible(v));
  function anyPlaying() {
    for (const v of seen) { if (!v.isConnected) seen.delete(v); else if (playing(v)) return true; }
    for (const v of document.getElementsByTagName('video')) if (playing(v)) return true;
    return false;
  }
  function pauseAll() {
    for (const v of [...seen, ...document.getElementsByTagName('video')]) { try { v.pause(); } catch { /* ignore */ } }
  }
  function report(force) {
    const now = Date.now();
    const state = anyPlaying();
    if (!force && state === lastState && (!state || now - lastSent < HEARTBEAT_MS)) return;
    lastState = state;
    lastSent = now;
    send({ type: 'kg:core:video', playing: state }).then((r) => {
      if (r && r.videoOver) { over = true; pauseAll(); }
      else if (r) over = false;
    });
  }
  const onMedia = (e) => {
    const v = e.target;
    if (!(v instanceof HTMLVideoElement)) return;
    seen.add(v);
    if (over && e.type === 'play') { v.pause(); }
    if (e.type === 'timeupdate') report(false);
    else setTimeout(() => report(true), 0);
  };
  for (const type of ['play', 'playing', 'pause', 'ended', 'timeupdate']) document.addEventListener(type, onMedia, true);
  window.addEventListener('pagehide', () => { if (lastState) send({ type: 'kg:core:video', playing: false }); });
})();
