package com.raisesteady.guard.lists

import java.io.DataInputStream
import java.io.IOException
import java.io.InputStream

/** The block categories in lists.bin (ids fixed by build-logic's SharedDataTask). */
enum class Category(val id: Int) {
    ADULT(1), GAMBLING(2), BYPASS(3), SOCIAL(4), DOH(5),

    /** Sites mainly about the topic filter's subject (shared/topics.json "domains"). */
    TOPIC(6);

    companion object {
        fun of(id: Int): Category? = entries.firstOrNull { it.id == id }
    }
}

/**
 * The packed block lists: per category a sorted array of FNV-1a 64 hashes of domains. A host
 * matches a category when the host or one of its parents (at least two labels) is listed.
 * About 1.4 MB in memory, loaded once; a lookup is a few binary searches.
 */
class DomainLists private constructor(private val sets: Map<Category, LongArray>) {

    fun count(c: Category): Int = sets[c]?.size ?: 0

    /** The first listed suffix of [host] in [c] (the host itself first), or null. */
    fun matchIn(c: Category, host: String): String? {
        val arr = sets[c] ?: return null
        var start = 0
        val end = host.length
        while (true) {
            val dot = host.indexOf('.', start)
            if (dot < 0) return null // a single label is never matched (not a registrable name)
            if (java.util.Arrays.binarySearch(arr, Fnv64.hash(host, start, end)) >= 0) return host.substring(start)
            start = dot + 1
        }
    }

    fun contains(c: Category, host: String): Boolean = matchIn(c, host) != null

    companion object {
        private const val MAGIC = 0x5253474C // "RSGL"

        @Throws(IOException::class)
        fun read(input: InputStream): DomainLists {
            DataInputStream(input.buffered(1 shl 16)).use { d ->
                if (d.readInt() != MAGIC) throw IOException("lists.bin: bad magic")
                val version = d.readInt()
                if (version != 1) throw IOException("lists.bin: version $version")
                val n = d.readInt()
                val sets = HashMap<Category, LongArray>()
                repeat(n) {
                    val id = d.readInt()
                    val count = d.readInt()
                    if (count < 0 || count > 2_000_000) throw IOException("lists.bin: bad count $count")
                    val arr = LongArray(count) { d.readLong() }
                    for (i in 1 until count) if (arr[i - 1] >= arr[i]) throw IOException("lists.bin: category $id not sorted")
                    Category.of(id)?.let { sets[it] = arr }
                }
                return DomainLists(sets)
            }
        }
    }
}
