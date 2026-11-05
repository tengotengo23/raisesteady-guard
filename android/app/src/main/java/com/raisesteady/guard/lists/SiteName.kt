package com.raisesteady.guard.lists

/**
 * Cleans an address the parent types: as Domains.normalize on iOS (guard-ios/Shared/State.swift)
 * and NormalizeDomain on Windows and Linux: lower case, no scheme, path, port, "www." or trailing
 * dot; "" when it is not a plain host name.
 */
object SiteName {
    private val host = Regex("^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\\.)+[a-z0-9-]{2,63}$")

    fun clean(raw: String): String {
        var s = raw.trim().lowercase()
        for (p in listOf("https://", "http://")) if (s.startsWith(p)) s = s.substring(p.length)
        val cut = s.indexOfFirst { it in "/?#:" }
        if (cut >= 0) s = s.substring(0, cut)
        s = s.removeSuffix(".")
        s = s.removePrefix("www.")
        return if (s.length <= 253 && host.matches(s)) s else ""
    }

    /** The DNS query name as the lists use it: lower case, no trailing dot. */
    fun queryName(name: String): String = name.lowercase().removeSuffix(".")
}
