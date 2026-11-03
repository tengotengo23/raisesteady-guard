// standalone PIN (PBKDF2-SHA256, 150 000 rounds) and the 5-tries lockout.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { hashPin, checkPinHash, guardedCheck, PIN_ROUNDS } from '../../ext/background/pin.js';

test('hash and check', async () => {
  const rec = await hashPin('2468');
  assert.equal(rec.rounds, PIN_ROUNDS);
  assert.equal(PIN_ROUNDS, 150000);
  assert.ok(!JSON.stringify(rec).includes('2468'));
  assert.equal(await checkPinHash('2468', rec), true);
  assert.equal(await checkPinHash('2469', rec), false);
  assert.equal(await checkPinHash('abc', rec), false);
  const again = await hashPin('2468');
  assert.notEqual(again.salt, rec.salt, 'random salt');
  await assert.rejects(hashPin('12'));
});

test('five wrong PINs lock checks for 15 minutes', async () => {
  let v = null;
  const store = { get: async () => v, set: async (x) => { v = x; } };
  const t0 = 1_000_000;
  for (let i = 0; i < 4; i++) assert.equal((await guardedCheck(store, t0, async () => false)).locked, false);
  const fifth = await guardedCheck(store, t0, async () => false);
  assert.equal(fifth.locked, true);
  assert.equal((await guardedCheck(store, t0 + 14 * 60e3, async () => true)).ok, false, 'right PIN still refused while locked');
  assert.equal((await guardedCheck(store, t0 + 15 * 60e3 + 1, async () => true)).ok, true);
});
