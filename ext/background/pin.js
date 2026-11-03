// Standalone parent PIN: PBKDF2-SHA256, 150 000 rounds, random 16-byte salt (WebCrypto).
// Five wrong PINs lock PIN checks for 15 minutes (same rule as the agent).

export const PIN_ROUNDS = 150000;
export const PIN_RE = /^\d{4,8}$/;
const MAX_FAILS = 5;
const LOCK_MS = 15 * 60 * 1000;

const b64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf)));
const unb64 = (s) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

async function derive(pin, salt, rounds) {
  const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(pin), 'PBKDF2', false, ['deriveBits']);
  return crypto.subtle.deriveBits({ name: 'PBKDF2', hash: 'SHA-256', salt, iterations: rounds }, key, 256);
}

export async function hashPin(pin) {
  if (!PIN_RE.test(String(pin))) throw new Error('pin-format');
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const hash = await derive(String(pin), salt, PIN_ROUNDS);
  return { alg: 'pbkdf2-sha256', rounds: PIN_ROUNDS, salt: b64(salt), hash: b64(hash) };
}

export async function checkPinHash(pin, rec) {
  if (!rec || !PIN_RE.test(String(pin))) return false;
  const got = new Uint8Array(await derive(String(pin), unb64(rec.salt), rec.rounds || PIN_ROUNDS));
  const want = unb64(rec.hash);
  if (got.length !== want.length) return false;
  let diff = 0;
  for (let i = 0; i < got.length; i++) diff |= got[i] ^ want[i];
  return diff === 0;
}

/** Wraps a check with the lockout; `store` = { get(): {fails, until}, set(v) }. Returns {ok, locked?}. */
export async function guardedCheck(store, nowMs, check) {
  const st = (await store.get()) || { fails: 0, until: 0 };
  if (st.until && nowMs < st.until) return { ok: false, locked: true, until: st.until };
  const ok = await check();
  if (ok) { await store.set({ fails: 0, until: 0 }); return { ok: true }; }
  const fails = (st.until && nowMs >= st.until ? 0 : st.fails || 0) + 1;
  const next = fails >= MAX_FAILS ? { fails: 0, until: nowMs + LOCK_MS } : { fails, until: 0 };
  await store.set(next);
  return { ok: false, locked: !!next.until, until: next.until || undefined };
}
