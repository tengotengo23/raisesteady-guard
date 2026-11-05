package com.raisesteady.guard.data

import com.raisesteady.guard.lists.YouTubeMode
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** One age band of shared/age-bands.json (the same bands on every system). */
@Serializable
data class AgeBand(
    val id: String,
    val minAge: Int,
    val maxAge: Int,
    val videoMin: Int,
    val bedtime: Window,
    val youtube: String,
    val social: String = "block",
) {
    val youtubeMode: YouTubeMode get() = if (youtube == "strict") YouTubeMode.STRICT else YouTubeMode.MODERATE
}

class AgeBands(val bands: List<AgeBand>) {
    fun byId(id: String): AgeBand = bands.firstOrNull { it.id == id } ?: bands.first()

    companion object {
        private val json = Json { ignoreUnknownKeys = true }

        @Serializable
        private data class File(val bands: List<AgeBand>)

        fun parse(text: String) = AgeBands(json.decodeFromString<File>(text).bands)
    }
}
