package com.raisesteady.guard.pin

import com.raisesteady.guard.data.PinLock
import com.raisesteady.guard.data.PinRecord
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.PBEKeySpec

/**
 * The parent PIN, as ext/background/pin.js and the agent: 4 to 8 digits, PBKDF2-SHA256 with
 * 150,000 rounds and a random 16-byte salt; five wrong PINs lock PIN checks for 15 minutes.
 * Pure except for the random salt; the lock is stored by the caller (state.json), so it survives
 * a restart.
 */
object Pin {
    const val ROUNDS = 150_000
    const val MAX_FAILS = 5
    const val LOCK_MS = 15 * 60 * 1000L
    private const val SALT_BYTES = 16
    private const val HASH_BITS = 256
    private val format = Regex("^\\d{4,8}$")

    fun valid(pin: String) = format.matches(pin)

    fun hash(pin: String, random: SecureRandom = SecureRandom(), rounds: Int = ROUNDS): PinRecord {
        require(valid(pin)) { "pin-format" }
        val salt = ByteArray(SALT_BYTES).also(random::nextBytes)
        val b64 = Base64.getEncoder()
        return PinRecord(rounds = rounds, salt = b64.encodeToString(salt), hash = b64.encodeToString(derive(pin, salt, rounds)))
    }

    fun matches(pin: String, rec: PinRecord?): Boolean {
        if (rec == null || !valid(pin) || rec.alg != "pbkdf2-sha256") return false
        val dec = Base64.getDecoder()
        val want = runCatching { dec.decode(rec.hash) }.getOrNull() ?: return false
        val got = derive(pin, dec.decode(rec.salt), rec.rounds)
        return MessageDigest.isEqual(got, want)
    }

    private fun derive(pin: String, salt: ByteArray, rounds: Int): ByteArray {
        val spec = PBEKeySpec(pin.toCharArray(), salt, rounds, HASH_BITS)
        try {
            return SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256").generateSecret(spec).encoded
        } finally {
            spec.clearPassword()
        }
    }

    /** The outcome of one PIN try. [until] is the end of the lock when [locked]. */
    data class Result(val ok: Boolean, val locked: Boolean = false, val until: Long = 0)

    /**
     * One try with the lockout (as guardedCheck in pin.js). While locked, the PIN is not even
     * checked. Returns the result and the lock to store.
     */
    fun check(pin: String, rec: PinRecord?, lock: PinLock, nowMs: Long): Pair<Result, PinLock> {
        if (lock.until != 0L && nowMs < lock.until) return Result(ok = false, locked = true, until = lock.until) to lock
        if (matches(pin, rec)) return Result(ok = true) to PinLock()
        val fails = (if (lock.until != 0L && nowMs >= lock.until) 0 else lock.fails) + 1
        val next = if (fails >= MAX_FAILS) PinLock(0, nowMs + LOCK_MS) else PinLock(fails, 0)
        return Result(ok = false, locked = next.until != 0L, until = next.until) to next
    }
}
