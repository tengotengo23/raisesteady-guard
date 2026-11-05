package com.raisesteady.guard.data

import kotlinx.serialization.Serializable

/** A clock window "HH:MM" to "HH:MM" (a bedtime may cross midnight; homework time may not). */
@Serializable
data class Window(val start: String, val end: String) {
    companion object {
        /** Minutes after midnight, or null when not "HH:MM" (00:00 to 23:59). */
        fun minutes(hm: String): Int? {
            val p = hm.split(':')
            if (p.size != 2 || p.any { it.length != 2 }) return null
            val h = p[0].toIntOrNull() ?: return null
            val m = p[1].toIntOrNull() ?: return null
            return if (h in 0..23 && m in 0..59) h * 60 + m else null
        }
    }
}

/** Weekend times (off by default): Saturday and Sunday minutes; Friday and Saturday nights bedtime. */
@Serializable
data class Weekend(val on: Boolean = false, val minutes: Int, val bedtime: Window)

/** Homework time (off by default): Monday to Friday, a same-day window of at least 15 minutes. */
@Serializable
data class Homework(val on: Boolean = false, val start: String = "16:00", val end: String = "18:00")

/** Per app: as pc-apps.json on Windows and Linux. */
@Serializable
enum class Level { ALLOWED, GAME_TIME, BLOCKED }

/** The parent PIN, as ext/background/pin.js: PBKDF2-SHA256, 150,000 rounds, 16-byte salt, 32-byte hash. */
@Serializable
data class PinRecord(val alg: String = "pbkdf2-sha256", val rounds: Int, val salt: String, val hash: String)

/** What the parent set (settings.json). Times are local wall-clock times. */
@Serializable
data class Settings(
    val setupDone: Boolean = false,
    /** shared/age-bands.json id: "3-5", "6-8", "9-12", "13-15", "16-17". */
    val bandId: String = "3-5",
    /** The daily Game time pool for every Game-time app, in minutes. */
    val gameMinutes: Int = 30,
    val bedtimeOn: Boolean = true,
    val bedtime: Window = Window("19:30", "07:00"),
    val weekend: Weekend = Weekend(on = false, minutes = 30, bedtime = Window("19:30", "07:00")),
    val homework: Homework = Homework(),
    /** AdGuard DNS Family upstream (on by default; off: the network's own DNS). */
    val familyDns: Boolean = true,
    /** The topic filter (sites about sexuality and gender identity), on for every age; the parent can switch it off. */
    val topicFilter: Boolean = true,
    /** The parent's lists (cleaned host names, at most [MAX_SITES] each). */
    val allow: List<String> = emptyList(),
    val block: List<String> = emptyList(),
    /** Levels the parent chose; apps without an entry use their default (AppCatalog). */
    val appLevels: Map<String, Level> = emptyMap(),
    val pin: PinRecord? = null,
) {
    companion object {
        const val MAX_SITES = 500
        const val MIN_GAME_MINUTES = 15
        const val MAX_GAME_MINUTES = 240
        const val GAME_MINUTES_STEP = 15

        /** The defaults of an age band (as Windows: game time starts at the band's video time). */
        fun forBand(band: AgeBand) = Settings(
            bandId = band.id,
            gameMinutes = band.videoMin,
            bedtime = band.bedtime,
            weekend = Weekend(on = false, minutes = band.videoMin, bedtime = band.bedtime),
        )
    }
}
