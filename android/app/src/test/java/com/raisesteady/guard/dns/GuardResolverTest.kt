package com.raisesteady.guard.dns

import com.raisesteady.guard.TestData
import com.raisesteady.guard.lists.YouTubeMode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.xbill.DNS.ARecord
import org.xbill.DNS.CNAMERecord
import org.xbill.DNS.DClass
import org.xbill.DNS.Flags
import org.xbill.DNS.Message
import org.xbill.DNS.Name
import org.xbill.DNS.Rcode
import org.xbill.DNS.Record
import org.xbill.DNS.Section
import org.xbill.DNS.Type
import java.net.InetAddress

/** The resolver with a fake upstream that answers every A question with 192.0.2.99 (TTL 120). */
class GuardResolverTest {
    private val asked = mutableListOf<String>()
    private var upstreamDown = false
    private var now = 1_000_000L

    private val fake = Upstream { raw, cb ->
        val q = Message(raw)
        asked += q.question.name.toString(true)
        if (upstreamDown) return@Upstream cb(null)
        val r = Message(q.header.id)
        r.header.setFlag(Flags.QR.toInt())
        r.addRecord(q.question, Section.QUESTION)
        r.addRecord(ARecord(q.question.name, DClass.IN, 120, InetAddress.getByName("192.0.2.99")), Section.ANSWER)
        cb(r.toWire())
    }

    private val resolver = GuardResolver(
        decider = DnsDecider(TestData.data.lists, TestData.data.social, TestData.data.helplines),
        settings = { FilterSettings(ageMin = 9, youtube = YouTubeMode.STRICT) },
        upstream = { fake },
        clock = { now },
    )

    private fun ask(name: String, type: Int = Type.A): Message {
        val q = Message.newQuery(Record.newRecord(Name.fromString("$name."), type, DClass.IN))
        var out: ByteArray? = null
        resolver.resolve(q.toWire()) { out = it }
        return Message(out!!)
    }

    @Test fun blockedNamesNeverReachTheUpstream() {
        assertEquals(Rcode.NXDOMAIN, ask("pornhub.com").header.rcode)
        assertEquals(Rcode.NXDOMAIN, ask("tiktok.com", Type.AAAA).header.rcode)
        assertTrue(asked.isEmpty())
    }

    @Test fun allowedNamesAreForwarded() {
        val r = ask("example.org")
        assertEquals(Rcode.NOERROR, r.header.rcode)
        assertEquals(listOf("example.org"), asked)
    }

    @Test fun upstreamFailureIsServfail() {
        upstreamDown = true
        assertEquals(Rcode.SERVFAIL, ask("example.org").header.rcode)
    }

    @Test fun safeSearchAsksForTheTargetAndCachesItForItsTtl() {
        val r = ask("www.google.com")
        val answers = r.getSection(Section.ANSWER)
        assertEquals("forcesafesearch.google.com.", (answers[0] as CNAMERecord).target.toString())
        assertEquals(Name.fromString("forcesafesearch.google.com."), answers[1].name)
        assertEquals(listOf("forcesafesearch.google.com"), asked)
        ask("google.ge")
        assertEquals(1, asked.size) // from the cache
        now += 121_000
        ask("google.ge")
        assertEquals(2, asked.size) // expired
    }

    @Test fun malformedMessagesGoUpstreamUnchanged() {
        val raw = byteArrayOf(1, 2, 3)
        var sent: ByteArray? = null
        val r = GuardResolver(
            decider = DnsDecider(TestData.data.lists, TestData.data.social, TestData.data.helplines),
            settings = { FilterSettings.SAFEST },
            upstream = { Upstream { m, cb -> sent = m; cb(null) } },
        )
        r.resolve(raw) { }
        assertTrue(raw.contentEquals(sent))
    }
}
