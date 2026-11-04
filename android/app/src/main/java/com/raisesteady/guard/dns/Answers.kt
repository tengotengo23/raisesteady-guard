package com.raisesteady.guard.dns

import org.xbill.DNS.CNAMERecord
import org.xbill.DNS.DClass
import org.xbill.DNS.Flags
import org.xbill.DNS.Message
import org.xbill.DNS.Name
import org.xbill.DNS.Rcode
import org.xbill.DNS.SOARecord
import org.xbill.DNS.Section
import org.xbill.DNS.Type

/** Builds the DNS answers the filter gives itself (dnsjava). Each answer keeps the query's id and question. */
object Answers {
    /** Negative answers live 60 s, so a site the parent allows opens within a minute. */
    const val NEGATIVE_TTL = 60L

    /** Rewritten answers live at most this long, and at least [MIN_TTL]. */
    const val MAX_TTL = 3600L
    const val MIN_TTL = 30L

    private val soaName: Name = Name.fromConstantString("blocked.raisesteady-guard.invalid.")
    private val soaAdmin: Name = Name.fromConstantString("hostmaster.raisesteady-guard.invalid.")
    private val soa = SOARecord(soaName, DClass.IN, NEGATIVE_TTL, soaName, soaAdmin, 1, 0, 0, 0, NEGATIVE_TTL)

    fun nxdomain(query: Message): ByteArray = base(query, Rcode.NXDOMAIN).also { it.addRecord(soa, Section.AUTHORITY) }.toWire()

    fun noData(query: Message): ByteArray = base(query, Rcode.NOERROR).also { it.addRecord(soa, Section.AUTHORITY) }.toWire()

    fun servfail(query: Message): ByteArray = base(query, Rcode.SERVFAIL).toWire()

    /**
     * The safe search answer: `name CNAME target`, then the target's own answer records (addresses and
     * any CNAME chain) from [targetAnswer]. A failed target lookup gives SERVFAIL.
     */
    fun cname(query: Message, target: Name, targetAnswer: Message): ByteArray {
        val rcode = targetAnswer.header.rcode
        if (rcode != Rcode.NOERROR) return base(query, if (rcode == Rcode.NXDOMAIN) Rcode.NXDOMAIN else Rcode.SERVFAIL).toWire()
        val records = targetAnswer.getSection(Section.ANSWER)
            .filter { it.type == Type.A || it.type == Type.AAAA || it.type == Type.CNAME }
        val ttl = (records.minOfOrNull { it.ttl } ?: MIN_TTL).coerceIn(MIN_TTL, MAX_TTL)
        val r = base(query, Rcode.NOERROR)
        r.addRecord(CNAMERecord(query.question.name, DClass.IN, ttl, target), Section.ANSWER)
        for (rec in records) r.addRecord(rec, Section.ANSWER)
        return r.toWire()
    }

    /** A reply with the query's id, question and RD flag; QR and RA set. */
    private fun base(query: Message, rcode: Int): Message {
        val r = Message(query.header.id)
        r.header.setFlag(Flags.QR.toInt())
        if (query.header.getFlag(Flags.RD.toInt())) r.header.setFlag(Flags.RD.toInt())
        r.header.setFlag(Flags.RA.toInt())
        r.header.rcode = rcode
        query.question?.let { r.addRecord(it, Section.QUESTION) }
        return r
    }
}
