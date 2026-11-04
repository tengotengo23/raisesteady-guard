package com.raisesteady.guard.rules

import java.time.Instant
import java.time.LocalDate

/** A usage event, as UsageStatsManager gives them (mapped from UsageEvents.Event types by the platform code). */
data class UsageEvent(val pkg: String, val cls: String, val kind: Kind, val timeMs: Long) {
    enum class Kind {
        /** ACTIVITY_RESUMED */
        RESUMED,

        /** ACTIVITY_PAUSED or ACTIVITY_STOPPED */
        PAUSED,

        /** SCREEN_NON_INTERACTIVE: every open interval ends */
        SCREEN_OFF,

        /** DEVICE_SHUTDOWN: every open interval ends */
        SHUTDOWN,
    }
}

/** Time an app was on screen: [startMs, endMs). */
data class Interval(val pkg: String, val startMs: Long, val endMs: Long) {
    val lengthMs get() = endMs - startMs
}

/**
 * Minutes from usage events, pure. An app's interval opens when its first activity is resumed and
 * closes when its last one is paused or stopped, or the screen goes off, or the device shuts down;
 * one still open counts to now. The Game time pool is the union of the Game-time apps' intervals,
 * so split screen does not count twice. Days are cut at local midnight (Schedule.day).
 */
object UsageMath {

    fun intervals(events: List<UsageEvent>, nowMs: Long): List<Interval> {
        val open = HashMap<String, MutableSet<String>>()
        val since = HashMap<String, Long>()
        val out = ArrayList<Interval>()
        fun close(pkg: String, t: Long) {
            val s = since.remove(pkg) ?: return
            open.remove(pkg)
            if (t > s) out += Interval(pkg, s, t)
        }
        for (e in events.sortedBy { it.timeMs }) {
            when (e.kind) {
                UsageEvent.Kind.RESUMED -> {
                    val set = open.getOrPut(e.pkg) { HashSet() }
                    if (set.isEmpty()) since[e.pkg] = e.timeMs
                    set += e.cls
                }
                UsageEvent.Kind.PAUSED -> {
                    val set = open[e.pkg] ?: continue
                    set -= e.cls
                    if (set.isEmpty()) close(e.pkg, e.timeMs)
                }
                UsageEvent.Kind.SCREEN_OFF, UsageEvent.Kind.SHUTDOWN -> for (p in since.keys.toList()) close(p, e.timeMs)
            }
        }
        for (p in since.keys.toList()) close(p, maxOf(nowMs, since[p] ?: nowMs))
        return out.sortedWith(compareBy({ it.startMs }, { it.pkg }))
    }

    /** Milliseconds per app inside [day]. */
    fun perApp(intervals: List<Interval>, day: Span): Map<String, Long> {
        val out = HashMap<String, Long>()
        for (i in intervals) {
            val ms = clip(i, day)
            if (ms > 0) out[i.pkg] = (out[i.pkg] ?: 0) + ms
        }
        return out
    }

    /** Milliseconds of the union of [pkgs]' intervals inside [day] (the Game time pool). */
    fun poolMs(intervals: List<Interval>, pkgs: Set<String>, day: Span): Long {
        val a = day.start.toEpochMilli()
        val b = day.end.toEpochMilli()
        val spans = intervals.filter { it.pkg in pkgs }
            .map { maxOf(it.startMs, a) to minOf(it.endMs, b) }
            .filter { it.second > it.first }
            .sortedBy { it.first }
        var total = 0L
        var curStart = -1L
        var curEnd = -1L
        for ((s, e) in spans) {
            if (s > curEnd) {
                if (curEnd > curStart) total += curEnd - curStart
                curStart = s
                curEnd = e
            } else if (e > curEnd) {
                curEnd = e
            }
        }
        if (curEnd > curStart) total += curEnd - curStart
        return total
    }

    /** Milliseconds per app per local date, for every day the intervals touch. */
    fun perDay(intervals: List<Interval>, schedule: Schedule): Map<LocalDate, Map<String, Long>> {
        val out = HashMap<LocalDate, MutableMap<String, Long>>()
        for (i in intervals) {
            var t = Instant.ofEpochMilli(i.startMs)
            val end = Instant.ofEpochMilli(i.endMs)
            while (t.isBefore(end)) {
                val day = schedule.day(t)
                val stop = if (day.end.isBefore(end)) day.end else end
                val ms = stop.toEpochMilli() - t.toEpochMilli()
                val m = out.getOrPut(schedule.date(t)) { HashMap() }
                m[i.pkg] = (m[i.pkg] ?: 0) + ms
                t = stop
            }
        }
        return out
    }

    private fun clip(i: Interval, day: Span): Long =
        maxOf(0L, minOf(i.endMs, day.end.toEpochMilli()) - maxOf(i.startMs, day.start.toEpochMilli()))

    fun minutes(ms: Long): Int = (ms / 60_000L).toInt()
}
