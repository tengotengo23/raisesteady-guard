package com.raisesteady.guard.dns

import com.raisesteady.guard.TestData
import com.raisesteady.guard.lists.SafeSearch
import com.raisesteady.guard.lists.YouTubeMode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.xbill.DNS.DClass
import org.xbill.DNS.Type

class DnsDeciderTest {
    private val data = TestData.data
    private val decider = DnsDecider(data.lists, data.social, data.helplines)

    /** Age band 9-12, as the done-test. */
    private val band9 = FilterSettings(ageMin = 9, youtube = YouTubeMode.STRICT)
    private val band13 = FilterSettings(ageMin = 13, youtube = YouTubeMode.MODERATE)
    private val band16 = FilterSettings(ageMin = 16, youtube = YouTubeMode.MODERATE)

    private fun d(name: String, s: FilterSettings = band9, type: Int = Type.A) = decider.decide(name, type, DClass.IN, s)
    private fun reason(name: String, s: FilterSettings = band9) = (d(name, s) as? Decision.Block)?.reason

    @Test fun categoryListsBlockEveryType() {
        assertEquals(BlockReason.ADULT, reason("pornhub.com"))
        assertEquals(BlockReason.ADULT, reason("WWW.PornHub.com."))
        assertEquals(BlockReason.GAMBLING, reason("www.bet365.com"))
        assertEquals(BlockReason.BYPASS, reason("protonvpn.com"))
        assertEquals(BlockReason.DOH, reason("dns.google"))
        assertTrue(decider.decide("pornhub.com", Type.HTTPS, DClass.IN, band9) is Decision.Block)
        assertTrue(decider.decide("pornhub.com", Type.TXT, DClass.IN, band16) is Decision.Block)
    }

    @Test fun ordinarySitesGoUpstream() {
        for (h in listOf("example.org", "en.wikipedia.org", "khanacademy.org", "mail.google.com")) assertEquals(h, Decision.Forward, d(h))
    }

    @Test fun malformedOrNotInternetClassGoesUpstream() {
        assertEquals(Decision.Forward, decider.decide("pornhub.com", Type.A, DClass.CH, band9))
        assertEquals(Decision.Forward, d(""))
        assertEquals(Decision.Forward, d("."))
        assertEquals(Decision.Forward, d("localhost"))
    }

    @Test fun firefoxCanaryGetsNoAnswer() {
        assertEquals(BlockReason.CANARY, reason("use-application-dns.net"))
        assertEquals(BlockReason.CANARY, reason("use-application-dns.net", band16))
    }

    @Test fun socialUnderItsMinimumAge() {
        assertEquals(BlockReason.SOCIAL, reason("tiktok.com", band9))
        assertEquals(BlockReason.SOCIAL, reason("static.xx.fbcdn.net", band9))
        assertEquals(Decision.Forward, d("tiktok.com", band13))
        assertEquals(BlockReason.SOCIAL, reason("vk.com", band13)) // VK and OK: 14 in their terms
        assertEquals(Decision.Forward, d("vk.com", band16))
    }

    @Test fun topicSitesAreTheirOwnCategoryWhileTheFilterIsOn() {
        for (s in listOf(band9, band13, band16)) {
            assertEquals(BlockReason.TOPIC, reason("glaad.org", s))
            assertEquals(BlockReason.TOPIC, reason("www.thepinknews.com", s))
        }
        assertEquals(Decision.Forward, d("glaad.org", band9.copy(topicFilter = false)))
        // The parent's Allow list opens one topic site.
        assertEquals(Decision.Forward, d("glaad.org", band9.copy(allow = setOf("glaad.org"))))
    }

    @Test fun helplinesAreNeverBlocked() {
        val everything = band9.copy(block = data.helplines.toSet())
        for (h in data.helplines) {
            assertEquals(h, Decision.Forward, d(h, everything))
            assertEquals(h, Decision.Forward, d("www.$h", everything))
            assertTrue(decider.isHelpline("www.$h"))
        }
        assertEquals(Decision.Forward, d("thetrevorproject.org")) // a helpline about the topic stays open
    }

    @Test fun parentListsAllowAndBlock() {
        assertEquals(Decision.Forward, d("www.pornhub.com", band9.copy(allow = setOf("pornhub.com"))))
        assertEquals(Decision.Forward, d("tiktok.com", band9.copy(allow = setOf("tiktok.com"))))
        assertEquals(BlockReason.PARENT, reason("games.example.org", band9.copy(block = setOf("example.org"))))
        // The parent's Block list wins over safe search (as the extension's rule order).
        assertEquals(BlockReason.PARENT, reason("www.youtube.com", band9.copy(block = setOf("youtube.com"))))
    }

    @Test fun safeSearchStaysOnForAllowedSites() {
        val s = band9.copy(allow = setOf("google.com"))
        assertEquals(Decision.Rewrite(SafeSearch.GOOGLE), d("www.google.com", s))
    }

    @Test fun safeSearchRewritesAddressesAndEmptiesOtherTypes() {
        assertEquals(Decision.Rewrite(SafeSearch.GOOGLE), d("www.google.com"))
        assertEquals(Decision.Rewrite(SafeSearch.GOOGLE), d("www.google.com", type = Type.AAAA))
        assertEquals(Decision.NoData, d("www.google.com", type = Type.HTTPS))
        assertEquals(Decision.Rewrite(SafeSearch.YOUTUBE_STRICT), d("youtubei.googleapis.com", band9))
        assertEquals(Decision.Rewrite(SafeSearch.YOUTUBE_MODERATE), d("www.youtube.com", band13))
    }
}
