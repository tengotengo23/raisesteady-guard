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
  let flagged = false;
  function check() {
    if (flagged || !globalThis.KGClassify) return;
    let res;
    try { res = globalThis.KGClassify.classify(collect()); } catch { return; }
    if (!res.block) return;
    flagged = true;
    document.documentElement.style.setProperty('visibility', 'hidden', 'important');
    for (const v of document.querySelectorAll('video, audio')) { try { v.pause(); } catch { /* ignore */ } }
    send({ type: 'kg:core:page-flag', reason: 'content', why: res.reason, score: res.score });
  }
  if (isTop && /^https?:$/.test(location.protocol)) {
    const start = () => { check(); setTimeout(check, 3000); };
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
