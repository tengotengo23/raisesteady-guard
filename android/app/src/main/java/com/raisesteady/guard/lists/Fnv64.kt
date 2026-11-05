package com.raisesteady.guard.lists

/**
 * FNV-1a, 64 bit, over the UTF-8 bytes of the lower-case name. build-logic's SharedDataTask has the
 * same code; both are checked against the published test vectors (DomainListsTest).
 */
object Fnv64 {
    private const val OFFSET = -0x340d631b7bdddcdbL // 0xcbf29ce484222325
    private const val PRIME = 0x100000001b3L

    fun hash(s: String): Long = hash(s, 0, s.length)

    /** Hash of s[start, end), without making a substring (the DNS path calls this for every suffix). */
    fun hash(s: String, start: Int, end: Int): Long {
        var h = OFFSET
        for (i in start until end) {
            val c = s[i]
            if (c.code < 0x80) {
                val b = if (c in 'A'..'Z') c.code + 32 else c.code
                h = (h xor b.toLong()) * PRIME
            } else {
                // Rare (names are punycode on the wire): fall back to the full UTF-8 path.
                return slow(s.substring(start, end))
            }
        }
        return h
    }

    private fun slow(s: String): Long {
        var h = OFFSET
        for (b in s.lowercase().toByteArray(Charsets.UTF_8)) h = (h xor (b.toLong() and 0xff)) * PRIME
        return h
    }
}
