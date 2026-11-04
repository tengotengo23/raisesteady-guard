package com.raisesteady.guard.lists

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SafeSearchTest {
    private val strict = YouTubeMode.STRICT

    @Test fun googleHostsGoToForceSafeSearch() {
        for (h in listOf("www.google.com", "google.com", "google.ge", "www.google.co.uk", "www.google.com.ge", "google.ru")) {
            assertEquals(h, SafeSearch.GOOGLE, SafeSearch.target(h, strict))
        }
    }

    @Test fun otherGoogleAndLookAlikeHostsAreNotRewritten() {
        for (h in listOf("mail.google.com", "google.evil.com", "googleapis.com", "www.google.example.com", "notgoogle.com", "docs.google.com")) {
            assertNull(h, SafeSearch.target(h, strict))
        }
    }

    @Test fun bingAndDuckDuckGo() {
        assertEquals(SafeSearch.BING, SafeSearch.target("www.bing.com", strict))
        assertEquals(SafeSearch.BING, SafeSearch.target("cn.bing.com", strict))
        assertEquals(SafeSearch.DUCKDUCKGO, SafeSearch.target("duckduckgo.com", strict))
        assertEquals(SafeSearch.DUCKDUCKGO, SafeSearch.target("lite.duckduckgo.com", strict))
        assertNull(SafeSearch.target("api.bing.com", strict))
    }

    @Test fun youTubeFollowsTheAgeBand() {
        // age-bands.json: 3-12 strict, 13-17 moderate
        assertEquals(SafeSearch.YOUTUBE_STRICT, SafeSearch.target("www.youtube.com", YouTubeMode.STRICT))
        assertEquals(SafeSearch.YOUTUBE_STRICT, SafeSearch.target("youtubei.googleapis.com", YouTubeMode.STRICT))
        assertEquals(SafeSearch.YOUTUBE_MODERATE, SafeSearch.target("m.youtube.com", YouTubeMode.MODERATE))
        assertEquals(SafeSearch.YOUTUBE_MODERATE, SafeSearch.target("www.youtube-nocookie.com", YouTubeMode.MODERATE))
        assertNull(SafeSearch.target("i.ytimg.com", YouTubeMode.STRICT))
    }
}
