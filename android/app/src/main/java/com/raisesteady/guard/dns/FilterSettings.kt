package com.raisesteady.guard.dns

import com.raisesteady.guard.data.AgeBands
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.lists.YouTubeMode

/**
 * What the DNS filter needs from the parent's settings. [ageMin] is the lowest age of the chosen
 * band (3, 6, 9, 13, 16): social sites are blocked when it is under the site's minimum age.
 * [allow] and [block] hold cleaned host names (SiteName.clean); each also covers its subdomains.
 */
data class FilterSettings(
    val ageMin: Int,
    val youtube: YouTubeMode,
    val topicFilter: Boolean = true,
    val familyDns: Boolean = true,
    val allow: Set<String> = emptySet(),
    val block: Set<String> = emptySet(),
) {
    companion object {
        /** Before setup: the youngest band (the safest defaults). */
        val SAFEST = FilterSettings(ageMin = 3, youtube = YouTubeMode.STRICT)

        /** From the stored settings and the age band they name. */
        fun from(s: Settings, bands: AgeBands): FilterSettings {
            val band = bands.byId(s.bandId)
            return FilterSettings(
                ageMin = band.minAge,
                youtube = band.youtubeMode,
                topicFilter = s.topicFilter,
                familyDns = s.familyDns,
                allow = s.allow.toSet(),
                block = s.block.toSet(),
            )
        }
    }
}
