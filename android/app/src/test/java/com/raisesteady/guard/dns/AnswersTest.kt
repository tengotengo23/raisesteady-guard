package com.raisesteady.guard.dns

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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
import org.xbill.DNS.SOARecord
import org.xbill.DNS.Section
import org.xbill.DNS.Type
import java.net.InetAddress

class AnswersTest {
    private fun query(name: String, type: Int = Type.A): Message =
        Message.newQuery(Record.newRecord(Name.fromString("$name."), type, DClass.IN)).also { it.header.id = 0x2468 }

    private fun flag(m: Message, f: Byte) = m.header.getFlag(f.toInt())

    private fun assertReplyTo(q: Message, r: Message) {
        assertEquals(q.header.id, r.header.id)
        assertTrue(flag(r, Flags.QR))
        assertTrue(flag(r, Flags.RA))
        assertEquals(flag(q, Flags.RD), flag(r, Flags.RD))
        assertEquals(q.question, r.question)
    }

    @Test fun nxdomainCarriesAnSoaWithTtl60() {
        val q = query("pornhub.com")
        val r = Message(Answers.nxdomain(q))
        assertReplyTo(q, r)
        assertEquals(Rcode.NXDOMAIN, r.header.rcode)
        assertEquals(0, r.getSection(Section.ANSWER).size)
        val soa = r.getSection(Section.AUTHORITY).single() as SOARecord
        assertEquals(60L, soa.ttl)
        assertEquals(60L, soa.minimum)
    }

    @Test fun noDataAndServfail() {
        val q = query("www.google.com", Type.HTTPS)
        val n = Message(Answers.noData(q))
        assertReplyTo(q, n)
        assertEquals(Rcode.NOERROR, n.header.rcode)
        assertEquals(0, n.getSection(Section.ANSWER).size)
        assertEquals(1, n.getSection(Section.AUTHORITY).size)
        val s = Message(Answers.servfail(q))
        assertReplyTo(q, s)
        assertEquals(Rcode.SERVFAIL, s.header.rcode)
    }

    @Test fun safeSearchAnswerIsCnamePlusTheTargetsAddresses() {
        val q = query("www.google.com")
        val target = Name.fromString("forcesafesearch.google.com.")
        val up = Message.newQuery(Record.newRecord(target, Type.A, DClass.IN))
        up.header.setFlag(Flags.QR.toInt())
        up.addRecord(ARecord(target, DClass.IN, 300, InetAddress.getByName("216.239.38.120")), Section.ANSWER)
        val r = Message(Answers.cname(q, target, up))
        assertReplyTo(q, r)
        assertEquals(Rcode.NOERROR, r.header.rcode)
        assertFalse(flag(r, Flags.AA))
        val answers = r.getSection(Section.ANSWER)
        val cname = answers[0] as CNAMERecord
        assertEquals(Name.fromString("www.google.com."), cname.name)
        assertEquals(target, cname.target)
        assertEquals(300L, cname.ttl)
        val a = answers[1] as ARecord
        assertEquals(target, a.name)
        assertArrayEquals(InetAddress.getByName("216.239.38.120").address, a.address.address)
    }

    @Test fun failedTargetLookupIsServfail() {
        val q = query("www.youtube.com")
        val target = Name.fromString("restrict.youtube.com.")
        val up = Message.newQuery(Record.newRecord(target, Type.A, DClass.IN)).also { it.header.rcode = Rcode.SERVFAIL }
        assertEquals(Rcode.SERVFAIL, Message(Answers.cname(q, target, up)).header.rcode)
    }
}
