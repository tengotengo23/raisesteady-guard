package com.raisesteady.guard.dns

import com.raisesteady.guard.vpn.adaway.DnsPacketProxy
import org.xbill.DNS.DClass
import org.xbill.DNS.Message
import org.xbill.DNS.Name
import org.xbill.DNS.Record
import org.xbill.DNS.Section
import java.io.IOException
import java.util.concurrent.ConcurrentHashMap
import java.util.function.Consumer

/**
 * Answers each DNS message from the tunnel: decides with [DnsDecider], answers blocks itself, sends
 * the rest to the upstream, and builds the safe search CNAME answers (the target's addresses are
 * cached for their TTL). Malformed messages go to the upstream unchanged.
 */
class GuardResolver(
    private val decider: DnsDecider,
    private val settings: () -> FilterSettings,
    private val upstream: (FilterSettings) -> Upstream,
    private val clock: () -> Long = System::currentTimeMillis,
    private val onBlocked: (Decision.Block, String) -> Unit = { _, _ -> },
) : DnsPacketProxy.Resolver {

    private class Cached(val answer: Message, val expiresAt: Long)

    private val cache = ConcurrentHashMap<String, Cached>()

    @Throws(IOException::class)
    override fun resolve(query: ByteArray, reply: Consumer<ByteArray>) {
        val s = settings()
        val up = upstream(s)
        val msg = try {
            Message(query)
        } catch (e: IOException) {
            up.query(query) { it?.let(reply::accept) }
            return
        }
        val question = msg.question
        if (question == null || msg.header.opcode != 0) {
            up.query(query) { it?.let(reply::accept) }
            return
        }
        val name = question.name.toString(true)
        when (val d = decider.decide(name, question.type, question.dClass, s)) {
            Decision.Forward -> up.query(query) { reply.accept(it ?: Answers.servfail(msg)) }
            is Decision.Block -> {
                onBlocked(d, name)
                reply.accept(Answers.nxdomain(msg))
            }
            Decision.NoData -> reply.accept(Answers.noData(msg))
            is Decision.Rewrite -> rewrite(msg, d.target, question.type, up, reply)
        }
    }

    private fun rewrite(msg: Message, target: String, type: Int, up: Upstream, reply: Consumer<ByteArray>) {
        val targetName = Name.fromString("$target.")
        val key = "$target/$type"
        val now = clock()
        cache[key]?.let { c ->
            if (c.expiresAt > now) return reply.accept(Answers.cname(msg, targetName, c.answer))
            cache.remove(key)
        }
        val q = Message.newQuery(Record.newRecord(targetName, type, DClass.IN))
        up.query(q.toWire()) { raw ->
            val answer = raw?.let { runCatching { Message(it) }.getOrNull() }
            if (answer == null) {
                reply.accept(Answers.servfail(msg))
            } else {
                val ttl = answer.getSection(Section.ANSWER).minOfOrNull { it.ttl } ?: 0L
                if (ttl > 0) cache[key] = Cached(answer, clock() + ttl.coerceAtMost(Answers.MAX_TTL) * 1000)
                reply.accept(Answers.cname(msg, targetName, answer))
            }
        }
    }
}
