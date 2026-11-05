package com.raisesteady.guard.dns

import com.raisesteady.guard.lists.Category
import com.raisesteady.guard.lists.DomainLists
import com.raisesteady.guard.lists.SafeSearch
import com.raisesteady.guard.lists.SiteName
import com.raisesteady.guard.lists.SocialAges
import org.xbill.DNS.DClass
import org.xbill.DNS.Type

/** Why a name gets no answer. */
enum class BlockReason { CANARY, PARENT, ADULT, GAMBLING, BYPASS, DOH, TOPIC, SOCIAL }

/** What to answer for one DNS question. */
sealed interface Decision {
    /** Ask the upstream and pass its answer on. */
    data object Forward : Decision

    /** NXDOMAIN with an SOA (TTL 60), for every query type. */
    data class Block(val reason: BlockReason, val listed: String) : Decision

    /** Safe search: a CNAME to [target] plus the target's addresses. */
    data class Rewrite(val target: String) : Decision

    /** NOERROR without answers (safe search hosts, types other than A and AAAA). */
    data object NoData : Decision
}

/**
 * The filter's decision, pure (no I/O, no clock). Order, highest first, as the extension's rule
 * priorities (ext/background/guard.js): helplines are never blocked; the Firefox canary gets no
 * answer; the parent's Block list; safe search (also for sites on the parent's Allow list); the
 * parent's Allow list; adult, gambling, bypass and encrypted-DNS lists; topic sites when the topic
 * filter is on; social sites under their minimum age; everything else goes upstream.
 */
class DnsDecider(
    private val lists: DomainLists,
    private val social: SocialAges,
    helplines: Collection<String>,
) {
    private val helplines: Set<String> = helplines.map { SiteName.queryName(it) }.toHashSet()

    fun decide(rawName: String, type: Int, dclass: Int, s: FilterSettings): Decision {
        if (dclass != DClass.IN) return Decision.Forward
        val name = SiteName.queryName(rawName)
        if (name.isEmpty() || name.indexOf('.') < 0) return Decision.Forward
        if (inSet(name, helplines) != null) return Decision.Forward
        if (name == CANARY) return Decision.Block(BlockReason.CANARY, CANARY)
        inSet(name, s.block)?.let { return Decision.Block(BlockReason.PARENT, it) }
        SafeSearch.target(name, s.youtube)?.let { target ->
            return if (type == Type.A || type == Type.AAAA) Decision.Rewrite(target) else Decision.NoData
        }
        if (inSet(name, s.allow) != null) return Decision.Forward
        for ((category, reason) in ALWAYS) {
            lists.matchIn(category, name)?.let { return Decision.Block(reason, it) }
        }
        if (s.topicFilter) lists.matchIn(Category.TOPIC, name)?.let { return Decision.Block(BlockReason.TOPIC, it) }
        lists.matchIn(Category.SOCIAL, name)?.let {
            if (s.ageMin < social.minAgeFor(name)) return Decision.Block(BlockReason.SOCIAL, it)
        }
        return Decision.Forward
    }

    /** True when [host] is a helpline (or under one): never blocked, never noticed. */
    fun isHelpline(host: String): Boolean = inSet(SiteName.queryName(host), helplines) != null

    companion object {
        /** Firefox turns its own DNS over HTTPS off when this name does not resolve. */
        const val CANARY = "use-application-dns.net"

        private val ALWAYS = listOf(
            Category.ADULT to BlockReason.ADULT,
            Category.GAMBLING to BlockReason.GAMBLING,
            Category.BYPASS to BlockReason.BYPASS,
            Category.DOH to BlockReason.DOH,
        )

        /** The first of [host] and its parents that is in [set], or null. */
        fun inSet(host: String, set: Set<String>): String? {
            if (set.isEmpty()) return null
            var start = 0
            while (true) {
                val candidate = host.substring(start)
                if (candidate in set) return candidate
                val dot = host.indexOf('.', start)
                if (dot < 0) return null
                start = dot + 1
            }
        }
    }
}
