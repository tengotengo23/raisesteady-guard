// The name the extension reports to the agent, per browser (P2-194: Yandex
// Browser has its own line on the parent page).
import test from 'node:test';
import assert from 'node:assert/strict';
import { browserName } from '../../ext/background/agent.js';

const CHROME = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36';

function nameFor(nav) {
  const old = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  Object.defineProperty(globalThis, 'navigator', { value: nav, configurable: true, writable: true });
  try {
    return browserName();
  } finally {
    if (old) Object.defineProperty(globalThis, 'navigator', old);
    else delete globalThis.navigator;
  }
}

test('each protected browser reports its own name', () => {
  assert.equal(nameFor({ userAgent: CHROME }), 'chrome');
  assert.equal(nameFor({ userAgent: 'Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0' }), 'firefox');
  assert.equal(nameFor({ userAgent: CHROME + ' Edg/131.0.0.0' }), 'edge');
  assert.equal(nameFor({ userAgent: CHROME, brave: {} }), 'brave');
  assert.equal(nameFor({ userAgent: CHROME.replace('Chrome/131.0.0.0', 'Chrome/128.0.0.0 YaBrowser/24.10.0.0') }), 'yandex');
  assert.equal(nameFor({ userAgent: CHROME, userAgentData: { brands: [{ brand: 'Chromium' }, { brand: 'Yandex' }] } }), 'yandex');
});

test('no navigator at all is still a name', () => {
  assert.equal(nameFor(undefined), 'chrome');
});
