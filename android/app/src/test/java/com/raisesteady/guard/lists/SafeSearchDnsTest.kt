package com.raisesteady.guard.lists

import com.raisesteady.guard.TestData
import com.raisesteady.guard.dns.Decision
import com.raisesteady.guard.dns.DnsDecider
import com.raisesteady.guard.dns.FilterSettings
import org.junit.Assert.assertEquals
import org.junit.Test
import org.xbill.DNS.DClass
import org.xbill.DNS.Type

/** SafeSearchTest's DNS part: an HTTPS (type 65) or SVCB record could carry normal Google addresses. */
class SafeSearchDnsTest {
    private val decider = DnsDecider(TestData.data.lists, TestData.data.social, TestData.data.helplines)

    @Test fun httpsAndSvcbRecordsOfSearchHostsGetNoData() {
        val s = FilterSettings(ageMin = 9, youtube = YouTubeMode.STRICT)
        for (h in listOf("www.google.com", "www.bing.com", "duckduckgo.com", "www.youtube.com")) {
            assertEquals(65, Type.HTTPS)
            assertEquals(h, Decision.NoData, decider.decide(h, Type.HTTPS, DClass.IN, s))
            assertEquals(h, Decision.NoData, decider.decide(h, Type.SVCB, DClass.IN, s))
        }
    }
}
