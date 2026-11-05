package com.raisesteady.guard.rules

import com.raisesteady.guard.rules.UsageEvent.Kind.PAUSED
import com.raisesteady.guard.rules.UsageEvent.Kind.RESUMED
import com.raisesteady.guard.rules.UsageEvent.Kind.SCREEN_OFF
import com.raisesteady.guard.rules.UsageEvent.Kind.SHUTDOWN
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.ZoneId

class UsageMathTest {
    private val zone = ZoneId.of("Asia/Tbilisi")
    private val sch = Schedule(zone)
    private fun t(s: String) = LocalDateTime.parse("2026-11-05T$s").atZone(zone).toInstant().toEpochMilli()
    private fun e(pkg: String, kind: UsageEvent.Kind, time: String, cls: String = "Main") = UsageEvent(pkg, "$pkg.$cls", kind, t(time))
    private val day = sch.day(Instant.ofEpochMilli(t("12:00")))
    private fun min(ms: Long?) = UsageMath.minutes(ms ?: 0)

    @Test fun resumedAndPausedPairs() {
        val i = UsageMath.intervals(listOf(e("a", RESUMED, "10:00"), e("a", PAUSED, "10:20"), e("a", RESUMED, "11:00"), e("a", PAUSED, "11:05")), t("12:00"))
        assertEquals(25, min(UsageMath.perApp(i, day)["a"]))
    }

    @Test fun anOpenIntervalCountsToNow() {
        val i = UsageMath.intervals(listOf(e("a", RESUMED, "10:00")), t("10:42"))
        assertEquals(42, min(UsageMath.perApp(i, day)["a"]))
    }

    @Test fun screenOffAndShutdownCloseEverything() {
        val i = UsageMath.intervals(listOf(e("a", RESUMED, "10:00"), e("b", RESUMED, "10:05"), e("x", SCREEN_OFF, "10:10"), e("c", RESUMED, "11:00"), e("x", SHUTDOWN, "11:30")), t("12:00"))
        val m = UsageMath.perApp(i, day)
        assertEquals(10, min(m["a"]))
        assertEquals(5, min(m["b"]))
        assertEquals(30, min(m["c"]))
    }

    @Test fun movingBetweenActivitiesOfOneAppKeepsItOpen() {
        // A paused, B resumed, A stopped: the app stays on screen.
        val i = UsageMath.intervals(listOf(e("a", RESUMED, "10:00", "A"), e("a", PAUSED, "10:10", "A"), e("a", RESUMED, "10:10", "B"), e("a", PAUSED, "10:11", "A"), e("a", PAUSED, "10:30", "B")), t("12:00"))
        assertEquals(30, min(UsageMath.perApp(i, day)["a"]))
    }

    @Test fun splitScreenCountsOnceInThePool() {
        val i = UsageMath.intervals(listOf(e("a", RESUMED, "10:00"), e("b", RESUMED, "10:10"), e("a", PAUSED, "10:30"), e("b", PAUSED, "10:40")), t("12:00"))
        assertEquals(40, min(UsageMath.poolMs(i, setOf("a", "b"), day)))
        assertEquals(30, min(UsageMath.poolMs(i, setOf("a"), day)))
        assertEquals(0, min(UsageMath.poolMs(i, setOf("c"), day)))
        val m = UsageMath.perApp(i, day)
        assertEquals(30, min(m["a"]))
        assertEquals(30, min(m["b"]))
    }

    @Test fun anIntervalAcrossMidnightIsSplitIntoTwoDays() {
        val start = LocalDateTime.parse("2026-11-05T23:40").atZone(zone).toInstant().toEpochMilli()
        val end = LocalDateTime.parse("2026-11-06T00:25").atZone(zone).toInstant().toEpochMilli()
        val i = UsageMath.intervals(listOf(UsageEvent("a", "a.Main", RESUMED, start), UsageEvent("a", "a.Main", PAUSED, end)), end)
        val perDay = UsageMath.perDay(i, sch)
        assertEquals(20, min(perDay[LocalDate.parse("2026-11-05")]?.get("a")))
        assertEquals(25, min(perDay[LocalDate.parse("2026-11-06")]?.get("a")))
        val nextDay = sch.day(Instant.ofEpochMilli(end))
        assertEquals(25, min(UsageMath.poolMs(i, setOf("a"), nextDay)))
    }

    @Test fun pausesWithoutResumeAreIgnored() {
        val i = UsageMath.intervals(listOf(e("a", PAUSED, "09:00"), e("a", RESUMED, "10:00"), e("a", PAUSED, "10:15")), t("12:00"))
        assertEquals(15, min(UsageMath.perApp(i, day)["a"]))
    }
}
