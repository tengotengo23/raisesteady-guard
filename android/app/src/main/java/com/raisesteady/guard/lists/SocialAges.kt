package com.raisesteady.guard.lists

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * Minimum ages of the social feeds (social.json, from shared/social-domains.json). A social
 * domain without its own entry (an asset domain such as fbcdn.net) uses [DEFAULT_MIN_AGE].
 * Blocked when the age band's lowest age is under the minimum age (as iOS Model.swift).
 */
class SocialAges(private val minAge: Map<String, Int>) {

    fun minAgeFor(host: String): Int {
        var start = 0
        while (true) {
            minAge[host.substring(start)]?.let { return it }
            val dot = host.indexOf('.', start)
            if (dot < 0) return DEFAULT_MIN_AGE
            start = dot + 1
        }
    }

    companion object {
        const val DEFAULT_MIN_AGE = 13

        private val json = Json { ignoreUnknownKeys = true }

        fun parse(text: String): SocialAges =
            SocialAges(json.decodeFromString<List<Entry>>(text).associate { it.domain.lowercase() to it.minAge })
    }

    @Serializable
    private data class Entry(val domain: String, val minAge: Int = DEFAULT_MIN_AGE)
}
