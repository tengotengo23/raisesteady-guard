package com.raisesteady.guard.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** One entry of shared/android-apps.json. */
@Serializable
data class AndroidApp(
    val id: String,
    val name: String,
    val packages: List<String>,
    val kind: String,
    val minAge: Int? = null,
)

/**
 * The default level of each app (design 2.4), pure. Games and video start on Game time; social
 * apps start Blocked under their minimum age and on Game time from it; chat apps start Blocked
 * under their minimum age and Allowed from it; bypass apps start Blocked at every age; any other
 * app in Android's game category starts on Game time; everything else, and YouTube Kids, stays
 * Allowed. "Under the minimum age" uses the band's lowest age, as the DNS filter (SocialAges).
 * The parent's own choice (Settings.appLevels) always wins.
 */
class AppCatalog(apps: List<AndroidApp>, private val youtubeKids: String?) {
    private val byPackage: Map<String, AndroidApp> = apps.flatMap { a -> a.packages.map { it to a } }.toMap()

    fun entry(pkg: String): AndroidApp? = byPackage[pkg]

    fun defaultLevel(pkg: String, isGame: Boolean, ageMin: Int): Level {
        if (pkg == youtubeKids) return Level.ALLOWED
        val a = byPackage[pkg] ?: return if (isGame) Level.GAME_TIME else Level.ALLOWED
        val under = ageMin < (a.minAge ?: DEFAULT_MIN_AGE)
        return when (a.kind) {
            KIND_GAME, KIND_VIDEO -> Level.GAME_TIME
            KIND_SOCIAL -> if (under) Level.BLOCKED else Level.GAME_TIME
            KIND_CHAT -> if (under) Level.BLOCKED else Level.ALLOWED
            KIND_BYPASS -> Level.BLOCKED
            else -> if (isGame) Level.GAME_TIME else Level.ALLOWED
        }
    }

    /** The level in force: the parent's choice, or the default. */
    fun level(pkg: String, isGame: Boolean, s: Settings, ageMin: Int): Level =
        s.appLevels[pkg] ?: defaultLevel(pkg, isGame, ageMin)

    companion object {
        const val DEFAULT_MIN_AGE = 13
        const val KIND_GAME = "game"
        const val KIND_VIDEO = "video"
        const val KIND_SOCIAL = "social"
        const val KIND_CHAT = "chat"
        const val KIND_BYPASS = "bypass"

        private val json = Json { ignoreUnknownKeys = true }

        @Serializable
        private data class File(val youtubeKids: String? = null, val apps: List<AndroidApp>)

        fun parse(text: String): AppCatalog = json.decodeFromString<File>(text).let { AppCatalog(it.apps, it.youtubeKids) }
    }
}
