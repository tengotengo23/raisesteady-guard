package com.raisesteady.guard.lists

/** YouTube Restricted Mode level of the age band (shared/age-bands.json "youtube"). */
enum class YouTubeMode { STRICT, MODERATE }

/**
 * Safe search by DNS: the search host is answered with a CNAME to the provider's safe host.
 * The host must match exactly (after lower case and without a trailing dot).
 * Sources: Google https://support.google.com/websearch/answer/186669 ;
 * Bing strict.bing.com ; DuckDuckGo https://duckduckgo.com/duckduckgo-help-pages/features/safe-search ;
 * YouTube https://knowledge.workspace.google.com/admin/youtube/control-youtube-content-available-to-users
 */
object SafeSearch {
    const val GOOGLE = "forcesafesearch.google.com"
    const val BING = "strict.bing.com"
    const val DUCKDUCKGO = "safe.duckduckgo.com"
    const val YOUTUBE_STRICT = "restrict.youtube.com"
    const val YOUTUBE_MODERATE = "restrictmoderate.youtube.com"

    /** As ext/rules/safesearch.json rule 1: google.com, google.ge, google.co.uk, www. too. */
    private val googleHost = Regex("^(www\\.)?google\\.[a-z]{2,3}(\\.[a-z]{2})?$")
    private val bing = setOf("www.bing.com", "bing.com", "cn.bing.com")
    private val duck = setOf("duckduckgo.com", "www.duckduckgo.com", "html.duckduckgo.com", "lite.duckduckgo.com")

    /** The YouTube app uses youtubei.googleapis.com, so Restricted Mode reaches the app too. */
    private val youtube = setOf(
        "www.youtube.com", "m.youtube.com", "youtubei.googleapis.com", "youtube.googleapis.com", "www.youtube-nocookie.com",
    )

    /** The safe host for [name] (lower case, no trailing dot), or null when [name] is not a search host. */
    fun target(name: String, mode: YouTubeMode): String? = when {
        name in youtube -> if (mode == YouTubeMode.STRICT) YOUTUBE_STRICT else YOUTUBE_MODERATE
        name in bing -> BING
        name in duck -> DUCKDUCKGO
        googleHost.matches(name) -> GOOGLE
        else -> null
    }
}
