package com.raisesteady.guard.rules

import com.raisesteady.guard.data.Homework
import com.raisesteady.guard.data.Level
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.data.State
import com.raisesteady.guard.data.Window
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId

class PolicyTest {
    private val zone = ZoneId.of("Asia/Tbilisi")
    private val sch = Schedule(zone)
    private val exempt = setOf("com.raisesteady.guard", "com.google.android.apps.nexuslauncher", "com.google.android.dialer")
    private val policy = Policy(sch, exempt)
    private val s = Settings(gameMinutes = 60, bedtimeOn = true, bedtime = Window("21:00", "07:00"),
        homework = Homework(on = true, start = "16:00", end = "18:00"))
    private fun at(t: String): Instant = LocalDateTime.parse(t).atZone(zone).toInstant()
    private fun local(i: Instant?) = i?.atZone(zone)?.toLocalDateTime()?.toString()
    private val min = 60_000L

    private fun game(now: String, used: Long = 0, st: State = State()) = policy.decide("game", Level.GAME_TIME, at(now), s, st, used)

    @Test fun eachReasonAndItsUntilTime() {
        // Thursday 2026-11-05.
        assertEquals(Verdict.Open, game("2026-11-05T12:00", 59 * min))
        val up = game("2026-11-05T12:00", 60 * min) as Verdict.Closed
        assertEquals(Reason.TIME_UP, up.reason)
        assertEquals("2026-11-06T00:00", local(up.until))
        val bed = game("2026-11-05T21:30") as Verdict.Closed
        assertEquals(Reason.BEDTIME, bed.reason)
        assertEquals("2026-11-06T07:00", local(bed.until))
        val hw = game("2026-11-05T16:30") as Verdict.Closed
        assertEquals(Reason.HOMEWORK, hw.reason)
        assertEquals("2026-11-05T18:00", local(hw.until))
        val pause = game("2026-11-05T12:00", st = State().withPause(at("2026-11-05T13:00").toEpochMilli())) as Verdict.Closed
        assertEquals(Reason.PAUSED, pause.reason)
        assertEquals("2026-11-05T13:00", local(pause.until))
    }

    @Test fun pauseComesBeforeBedtimeAndBedtimeBeforeTimeUp() {
        val st = State().withPause(at("2026-11-06T06:00").toEpochMilli())
        assertEquals(Reason.PAUSED, (game("2026-11-05T22:00", 0, st) as Verdict.Closed).reason)
        assertEquals(Reason.BEDTIME, (game("2026-11-05T22:00", 120 * min) as Verdict.Closed).reason)
    }

    @Test fun moreTimeOpensGameTimeAppsButNotBlockedOnes() {
        val now = at("2026-11-05T21:30")
        val st = State().withMoreTime(15, now.toEpochMilli())
        assertEquals(Verdict.Open, policy.decide("game", Level.GAME_TIME, now, s, st, 500 * min))
        assertEquals(Verdict.Closed(Reason.BLOCKED, null), policy.decide("tor", Level.BLOCKED, now, s, st, 0))
        // 16 minutes later bedtime applies again.
        assertEquals(Reason.BEDTIME, (policy.decide("game", Level.GAME_TIME, now.plusSeconds(16 * 60), s, st, 0) as Verdict.Closed).reason)
    }

    @Test fun allowedAppsAreNeverClosed() {
        for (t in listOf("2026-11-05T12:00", "2026-11-05T16:30", "2026-11-05T23:00")) {
            assertEquals(Verdict.Open, policy.decide("maps", Level.ALLOWED, at(t), s, State().withPause(at("2026-11-06T06:00").toEpochMilli()), 999 * min))
        }
    }

    @Test fun guardTheLauncherAndTheDialerAreNeverClosed() {
        for (pkg in exempt) {
            for (level in Level.entries) {
                assertEquals(pkg, Verdict.Open, policy.decide(pkg, level, at("2026-11-05T23:00"), s, State(), 999 * min))
            }
        }
    }

    @Test fun nextCheckIsTheEarliestChange() {
        // 12:00 with 50 of 60 minutes used: the pool runs out at 12:10.
        assertEquals("2026-11-05T12:10", local(policy.nextCheck(at("2026-11-05T12:00"), s, State(), 50 * min)))
        // 15:50, pool fine: homework starts at 16:00.
        assertEquals("2026-11-05T16:00", local(policy.nextCheck(at("2026-11-05T15:50"), s, State(), 0)))
        // 20:30 after homework: bedtime at 21:00.
        assertEquals("2026-11-05T21:00", local(policy.nextCheck(at("2026-11-05T20:30"), s, State(), 0)))
        // More time ends first.
        val st = State().withMoreTime(15, at("2026-11-05T12:00").toEpochMilli())
        assertEquals("2026-11-05T12:15", local(policy.nextCheck(at("2026-11-05T12:00"), s, st, 0)))
    }
}
