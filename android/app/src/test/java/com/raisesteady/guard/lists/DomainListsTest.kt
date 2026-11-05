package com.raisesteady.guard.lists

import com.raisesteady.guard.TestData
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.BeforeClass
import org.junit.Test

class DomainListsTest {
    companion object {
        lateinit var lists: DomainLists
        lateinit var social: SocialAges

        @BeforeClass
        @JvmStatic
        fun load() {
            val t0 = System.nanoTime()
            lists = TestData.asset("lists.bin").inputStream().use { DomainLists.read(it) }
            println("lists.bin loaded in ${(System.nanoTime() - t0) / 1_000_000} ms")
            social = SocialAges.parse(TestData.asset("social.json").readText())
        }

        private fun json(rel: String) = Json.parseToJsonElement(TestData.kidguard.resolve(rel).readText())

        private fun ruleDomains(name: String): Set<String> =
            (json("ext/rules/$name.json") as JsonArray).flatMap { r ->
                r.jsonObject["condition"]!!.jsonObject["requestDomains"]?.jsonArray?.map { it.jsonPrimitive.content } ?: emptyList()
            }.toSet()

        private fun strings(rel: String, key: String): List<String> =
            (json(rel) as JsonObject)[key]!!.jsonArray.map { it.jsonPrimitive.content }
    }

    @Test fun fnvMatchesPublishedVectors() {
        assertEquals(0xcbf29ce484222325uL.toLong(), Fnv64.hash(""))
        assertEquals(0xaf63dc4c8601ec8cuL.toLong(), Fnv64.hash("a"))
        assertEquals(0x85944171f73967e8uL.toLong(), Fnv64.hash("foobar"))
        assertEquals(Fnv64.hash("foobar"), Fnv64.hash("FooBar"))
        assertEquals(Fnv64.hash("example.com"), Fnv64.hash("www.example.com", 4, 15))
        // Non-ASCII takes the UTF-8 path; it must equal hashing the lower-case UTF-8 bytes.
        assertEquals(Fnv64.hash("ü.de"), Fnv64.hash("Ü.de"))
    }

    @Test fun adultSites() {
        assertEquals("pornhub.com", lists.matchIn(Category.ADULT, "pornhub.com"))
        assertEquals("xvideos.com", lists.matchIn(Category.ADULT, "sub.xvideos.com"))
        assertTrue(lists.contains(Category.ADULT, "www.pornhub.com"))
    }

    @Test fun bypassAndGambling() {
        assertTrue(lists.contains(Category.BYPASS, "protonvpn.com"))
        assertTrue(lists.contains(Category.BYPASS, "croxyproxy.com"))
        assertTrue(lists.contains(Category.GAMBLING, "www.bet365.com"))
    }

    @Test fun socialWithMinimumAges() {
        assertTrue(lists.contains(Category.SOCIAL, "tiktok.com"))
        assertTrue(lists.contains(Category.SOCIAL, "m.vk.com"))
        assertTrue(lists.contains(Category.SOCIAL, "static.xx.fbcdn.net"))
        assertEquals(13, social.minAgeFor("www.tiktok.com"))
        assertEquals(14, social.minAgeFor("vk.com"))
        assertEquals(14, social.minAgeFor("m.ok.ru"))
        assertEquals(SocialAges.DEFAULT_MIN_AGE, social.minAgeFor("fbcdn.net"))
    }

    @Test fun encryptedDnsServers() {
        assertTrue(lists.contains(Category.DOH, "dns.google"))
        assertTrue(lists.contains(Category.DOH, "cloudflare-dns.com"))
        assertTrue(lists.contains(Category.DOH, "mozilla.cloudflare-dns.com"))
        assertTrue(lists.contains(Category.DOH, "use-application-dns.net"))
        // Our own upstream and its parent must stay reachable.
        assertNull(lists.matchIn(Category.DOH, "family.adguard-dns.com"))
    }

    @Test fun topicSites() {
        assertTrue(lists.contains(Category.TOPIC, "glaad.org"))
        assertTrue(lists.contains(Category.TOPIC, "www.thepinknews.com"))
        assertEquals(strings("shared/topics.json", "domains").toSet().size, lists.count(Category.TOPIC))
    }

    @Test fun helplinesAndOrdinarySitesAreInNoList() {
        val helplines = strings("shared/topics.json", "helplines")
        assertTrue(helplines.contains("findahelpline.com"))
        for (host in helplines + listOf("example.org", "wikipedia.org", "en.wikipedia.org", "khanacademy.org")) {
            for (c in Category.entries) assertNull("$host in $c", lists.matchIn(c, host))
        }
    }

    @Test fun singleLabelsNeverMatch() {
        for (c in Category.entries) {
            assertFalse(lists.contains(c, "com"))
            assertFalse(lists.contains(c, ""))
        }
    }

    @Test fun countsEqualTheSourceData() {
        assertEquals(110_000, lists.count(Category.ADULT))
        assertEquals(35_000, lists.count(Category.GAMBLING))
        assertEquals(25_000, lists.count(Category.BYPASS))
        assertEquals(ruleDomains("adult").size, lists.count(Category.ADULT))
        assertEquals(ruleDomains("gambling").size, lists.count(Category.GAMBLING))
        assertEquals(ruleDomains("bypass").size, lists.count(Category.BYPASS))
        val feeds = (json("shared/social-domains.json") as JsonObject)["feeds"]!!.jsonArray
            .map { it.jsonObject["domain"]!!.jsonPrimitive.content }
        assertEquals((ruleDomains("social") + feeds).size, lists.count(Category.SOCIAL))
        assertEquals(strings("shared/doh.json", "domains").toSet().size, lists.count(Category.DOH))
        val ips = Json.parseToJsonElement(TestData.asset("doh-ips.json").readText()).jsonArray
        assertEquals(strings("shared/doh.json", "ips").size, ips.size)
        assertTrue(ips.size > 1000)
    }

    @Test fun lookupIsFast() {
        val hosts = listOf("a.b.c.example.co.uk", "www.pornhub.com", "news.bbc.co.uk", "x.y.z.w.v.example.org")
        val t0 = System.nanoTime()
        var hits = 0
        repeat(10_000) { for (h in hosts) for (c in Category.entries) if (lists.contains(c, h)) hits++ }
        val perLookupNs = (System.nanoTime() - t0) / (10_000.0 * hosts.size * Category.entries.size)
        println("lookup: %.0f ns per host and category".format(perLookupNs))
        assertTrue(hits > 0)
        assertTrue("lookup too slow: $perLookupNs ns", perLookupNs < 50_000)
    }
}
