package com.raisesteady.guard.data

import kotlinx.serialization.Serializable

/** One local day of the report: no site names are kept. */
@Serializable
data class DayTotals(
    /** Minutes of the Game time pool used (the union of Game-time apps). */
    val poolMinutes: Int = 0,
    /** Minutes per app, the 10 most used. */
    val apps: Map<String, Int> = emptyMap(),
    val blockedLookups: Int = 0,
)

/** usage.json: per local day ("2026-11-05"), at most [KEEP_DAYS] days. */
@Serializable
data class Usage(val days: Map<String, DayTotals> = emptyMap()) {
    /** Puts [day] in and drops days older than [KEEP_DAYS] before [today] (ISO dates sort as text). */
    fun with(day: String, totals: DayTotals, today: java.time.LocalDate): Usage {
        val oldest = today.minusDays(KEEP_DAYS - 1L).toString()
        val kept = (days + (day to totals.copy(apps = top(totals.apps)))).filterKeys { it >= oldest }
        return Usage(kept.toSortedMap())
    }

    companion object {
        const val KEEP_DAYS = 30
        const val TOP_APPS = 10

        fun top(apps: Map<String, Int>): Map<String, Int> =
            apps.entries.sortedWith(compareByDescending<Map.Entry<String, Int>> { it.value }.thenBy { it.key })
                .take(TOP_APPS).associate { it.key to it.value }
    }
}
