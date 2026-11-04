package com.raisesteady.guard.rules

import com.raisesteady.guard.data.Level
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.data.State
import java.time.Instant

/** Why an app is closed (the block screen says it). */
enum class Reason { PAUSED, BEDTIME, HOMEWORK, TIME_UP, BLOCKED }

sealed interface Verdict {
    data object Open : Verdict

    /** [until] is when it opens again by itself, or null (Blocked: only the parent opens it). */
    data class Closed(val reason: Reason, val until: Instant?) : Verdict
}

/**
 * Whether an app may be on screen now, pure. Never limited: the packages in [exempt] (Guard itself,
 * the launcher, System UI, the keyboard, the dialer, SMS, Settings, the permission controller).
 * Blocked apps stay closed (more time does not open them). Game-time apps: more time opens them
 * whatever else applies; otherwise pause, then bedtime, then homework time, then the used-up pool
 * (the extension's order: paused > bedtime > homework > time).
 */
class Policy(private val schedule: Schedule, private val exempt: Set<String>) {

    fun decide(pkg: String, level: Level, now: Instant, s: Settings, st: State, poolUsedMs: Long): Verdict {
        if (pkg in exempt) return Verdict.Open
        return when (level) {
            Level.ALLOWED -> Verdict.Open
            Level.BLOCKED -> Verdict.Closed(Reason.BLOCKED, null)
            Level.GAME_TIME -> gameTime(now, s, st, poolUsedMs)
        }
    }

    private fun gameTime(now: Instant, s: Settings, st: State, poolUsedMs: Long): Verdict {
        val ms = now.toEpochMilli()
        if (st.moreTime(ms)) return Verdict.Open
        if (st.paused(ms)) return Verdict.Closed(Reason.PAUSED, Instant.ofEpochMilli(st.pauseUntil))
        schedule.bedtimeNow(s, now)?.let { return Verdict.Closed(Reason.BEDTIME, it.end) }
        schedule.homeworkNow(s, now)?.let { return Verdict.Closed(Reason.HOMEWORK, it.end) }
        if (poolUsedMs >= schedule.dailyMinutes(s, now) * 60_000L) return Verdict.Closed(Reason.TIME_UP, schedule.nextDayStart(now))
        return Verdict.Open
    }

    /**
     * When a Game-time app that is open now must be checked again: the earliest of the pool running
     * out, more time ending, a pause starting or ending, bedtime or homework starting, and midnight.
     */
    fun nextCheck(now: Instant, s: Settings, st: State, poolUsedMs: Long): Instant {
        val ms = now.toEpochMilli()
        val left = schedule.dailyMinutes(s, now) * 60_000L - poolUsedMs
        val times = listOfNotNull(
            if (left > 0) now.plusMillis(left) else null,
            if (st.moreTime(ms)) Instant.ofEpochMilli(st.moreTimeUntil) else null,
            if (st.paused(ms)) Instant.ofEpochMilli(st.pauseUntil) else null,
            schedule.nextBedtimeStart(s, now),
            schedule.nextHomeworkStart(s, now),
            schedule.nextDayStart(now),
        )
        return times.min()
    }
}
