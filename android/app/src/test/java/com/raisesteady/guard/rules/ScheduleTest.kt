package com.raisesteady.guard.rules

import com.raisesteady.guard.data.Homework
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.data.Weekend
import com.raisesteady.guard.data.Window
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Duration
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId

class ScheduleTest {
    private val tbilisi = ZoneId.of("Asia/Tbilisi")
    private val london = ZoneId.of("Europe/London")
    private val sch = Schedule(tbilisi)

    private fun at(s: String, zone: ZoneId = tbilisi): Instant = LocalDateTime.parse(s).atZone(zone).toInstant()
    private fun local(i: Instant?, zone: ZoneId = tbilisi) = i?.atZone(zone)?.toLocalDateTime()?.toString()

    private val base = Settings(bedtimeOn = true, bedtime = Window("21:00", "07:00"), gameMinutes = 60,
        weekend = Weekend(on = false, minutes = 180, bedtime = Window("22:30", "08:30")))
    private val weekendOn = base.copy(weekend = base.weekend.copy(on = true))

    @Test fun bedtimeAcrossMidnight() {
        // 2026-11-05 is a Thursday.
        assertNull(sch.bedtimeNow(base, at("2026-11-05T20:59")))
        assertEquals("2026-11-06T07:00", local(sch.bedtimeNow(base, at("2026-11-05T21:00"))?.end))
        assertEquals("2026-11-06T07:00", local(sch.bedtimeNow(base, at("2026-11-06T03:00"))?.end))
        assertNull(sch.bedtimeNow(base, at("2026-11-06T07:00")))
        assertNull(sch.bedtimeNow(base.copy(bedtimeOn = false), at("2026-11-05T23:00")))
    }

    @Test fun weekendDaysAndNights() {
        // Saturday and Sunday use the weekend minutes; weekdays the normal ones.
        assertEquals(180, sch.dailyMinutes(weekendOn, at("2026-11-07T10:00")))
        assertEquals(180, sch.dailyMinutes(weekendOn, at("2026-11-08T10:00")))
        assertEquals(60, sch.dailyMinutes(weekendOn, at("2026-11-06T10:00")))
        assertEquals(60, sch.dailyMinutes(base, at("2026-11-07T10:00")))
        // Friday 22:00 is a weekend night (bedtime 22:30): still open.
        assertNull(sch.bedtimeNow(weekendOn, at("2026-11-06T22:00")))
        assertEquals("2026-11-07T08:30", local(sch.bedtimeNow(weekendOn, at("2026-11-06T22:45"))?.end))
        // Saturday 08:00 is still Friday's weekend night.
        assertNotNull(sch.bedtimeNow(weekendOn, at("2026-11-07T08:00")))
        // Sunday 22:00 is a school night (bedtime 21:00, ends Monday 07:00).
        assertEquals("2026-11-09T07:00", local(sch.bedtimeNow(weekendOn, at("2026-11-08T22:00"))?.end))
        // Monday 07:30: the school night is over, though a weekend bedtime would run to 08:30.
        assertNull(sch.bedtimeNow(weekendOn, at("2026-11-09T07:30")))
    }

    @Test fun aBedtimeAfterMidnightBelongsToTheEveningBefore() {
        val s = base.copy(weekend = Weekend(on = true, minutes = 180, bedtime = Window("01:30", "09:00")))
        // Saturday 01:45 morning: Friday night, a weekend night: the weekend bedtime (01:30-09:00) applies.
        val night = sch.bedtimeNow(s, at("2026-11-07T01:45"))
        assertEquals("2026-11-07T09:00", local(night?.end))
        assertEquals(DayKind.WEEKEND, sch.nightKind(s, at("2026-11-07T01:30")))
        // Friday 01:45 morning: Thursday night, a school night: the school bedtime applies (ends 07:00).
        assertEquals("2026-11-06T07:00", local(sch.bedtimeNow(s, at("2026-11-06T01:45"))?.end))
        assertEquals(DayKind.SCHOOL, sch.nightKind(s, at("2026-11-06T01:30")))
    }

    @Test fun homeworkMondayToFridayOnly() {
        val s = base.copy(homework = Homework(on = true, start = "16:00", end = "18:00"))
        assertEquals("2026-11-06T18:00", local(sch.homeworkNow(s, at("2026-11-06T16:30"))?.end))
        assertNull(sch.homeworkNow(s, at("2026-11-06T18:00")))
        assertNull(sch.homeworkNow(s, at("2026-11-07T16:30"))) // Saturday
        assertNull(sch.homeworkNow(s, at("2026-11-08T16:30"))) // Sunday
        assertNull(sch.homeworkNow(s.copy(homework = s.homework.copy(on = false)), at("2026-11-06T16:30")))
        assertEquals("2026-11-09T16:00", local(sch.nextHomeworkStart(s, at("2026-11-06T19:00"))))
    }

    @Test fun windowsUnder15MinutesAreRefused() {
        assertTrue(Schedule.validHomework("16:00", "16:15"))
        assertFalse(Schedule.validHomework("16:00", "16:14"))
        assertFalse(Schedule.validHomework("18:00", "16:00")) // homework may not cross midnight
        assertTrue(Schedule.validNight("23:50", "00:05"))
        assertFalse(Schedule.validNight("23:50", "00:04"))
        assertFalse(Schedule.validNight("25:00", "07:00"))
        val short = base.copy(homework = Homework(on = true, start = "16:00", end = "16:10"))
        assertNull(sch.homeworkNow(short, at("2026-11-06T16:05")))
    }

    @Test fun pauseUntil6AcrossMidnight() {
        assertEquals("2026-11-06T06:00", local(sch.pauseEnd(PauseChoice.UNTIL_6, at("2026-11-05T22:00"))))
        assertEquals("2026-11-06T06:00", local(sch.pauseEnd(PauseChoice.UNTIL_6, at("2026-11-06T02:00"))))
        assertEquals("2026-11-05T22:30", local(sch.pauseEnd(PauseChoice.MIN_30, at("2026-11-05T22:00"))))
        assertEquals("2026-11-06T00:00", local(sch.pauseEnd(PauseChoice.HOUR_2, at("2026-11-05T22:00"))))
    }

    @Test fun moreTimeEndsAPauseForGood() {
        val now = at("2026-11-05T17:00").toEpochMilli()
        val paused = com.raisesteady.guard.data.State().withPause(at("2026-11-05T19:00").toEpochMilli())
        val more = paused.withMoreTime(15, now)
        assertFalse(more.paused(now))
        assertTrue(more.moreTime(now))
        // After the 15 minutes, the pause does not come back.
        assertFalse(more.paused(now + 16 * 60_000))
        assertFalse(more.moreTime(now + 16 * 60_000))
    }

    @Test fun daylightSavingDaysHave23And25Hours() {
        val uk = Schedule(london)
        assertEquals(Duration.ofHours(23), Duration.between(uk.dayStart(at("2026-03-29T12:00", london)), uk.nextDayStart(at("2026-03-29T12:00", london))))
        assertEquals(Duration.ofHours(25), Duration.between(uk.dayStart(at("2026-10-25T12:00", london)), uk.nextDayStart(at("2026-10-25T12:00", london))))
        // Bedtime 21:00 to 07:00 over both nights: the wall-clock end stays 07:00.
        val spring = uk.bedtimeNow(base, at("2026-03-29T06:30", london))!!
        assertEquals("2026-03-29T07:00", local(spring.end, london))
        assertEquals(Duration.ofHours(9), Duration.between(spring.start, spring.end))
        val autumn = uk.bedtimeNow(base, Instant.parse("2026-10-25T01:30:00Z"))!! // 01:30 GMT, after the change
        assertEquals("2026-10-25T07:00", local(autumn.end, london))
        assertEquals(Duration.ofHours(11), Duration.between(autumn.start, autumn.end))
    }

    @Test fun usageAcrossTheChangedHourCountsRealMinutes() {
        val uk = Schedule(london)
        // 25 Oct 2026: 00:30 BST (23:30 UTC on the 24th) to 02:30 GMT (02:30 UTC): 3 real hours.
        val autumn = Interval("game", Instant.parse("2026-10-24T23:30:00Z").toEpochMilli(), Instant.parse("2026-10-25T02:30:00Z").toEpochMilli())
        val day25 = uk.day(at("2026-10-25T12:00", london))
        assertEquals(180, UsageMath.minutes(UsageMath.poolMs(listOf(autumn), setOf("game"), day25)))
        // 29 Mar 2026: 00:30 GMT to 02:30 BST is 1 real hour.
        val spring = Interval("game", Instant.parse("2026-03-29T00:30:00Z").toEpochMilli(), Instant.parse("2026-03-29T01:30:00Z").toEpochMilli())
        assertEquals(60, UsageMath.minutes(UsageMath.perApp(listOf(spring), uk.day(at("2026-03-29T12:00", london)))["game"]!!))
    }
}
