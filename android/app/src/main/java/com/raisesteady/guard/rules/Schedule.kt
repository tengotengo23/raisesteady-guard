package com.raisesteady.guard.rules

import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.data.Window
import java.time.DayOfWeek
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

/** Which daily time and which bedtime apply. */
enum class DayKind { SCHOOL, WEEKEND }

/** A span of time [start, end). */
data class Span(val start: Instant, val end: Instant) {
    operator fun contains(t: Instant) = !t.isBefore(start) && t.isBefore(end)
}

/** Pause now: 30 min, 1 hour, 2 hours, or until 06:00 (the next one). */
enum class PauseChoice { MIN_30, HOUR_1, HOUR_2, UNTIL_6 }

/**
 * The time rules, pure (as guard-ios/Shared/Rules.swift and Windows/Linux F1-F3), on the local wall
 * clock of [zone]. A day is [start of the local date, start of the next local date), so 23- and
 * 25-hour days count right. Weekend times: Saturday and Sunday use the weekend minutes; Friday and
 * Saturday nights use the weekend bedtime; a night belongs to the day it starts, and a bedtime that
 * starts after midnight belongs to the evening before. Homework time: Monday to Friday, same day.
 */
class Schedule(val zone: ZoneId) {

    fun date(now: Instant): LocalDate = now.atZone(zone).toLocalDate()
    fun dayStart(now: Instant): Instant = date(now).atStartOfDay(zone).toInstant()
    fun nextDayStart(now: Instant): Instant = date(now).plusDays(1).atStartOfDay(zone).toInstant()
    fun day(now: Instant) = Span(dayStart(now), nextDayStart(now))

    /** "HH:MM" on [day] (a time in a daylight-saving gap moves forward by the gap, as java.time does). */
    fun clockTime(hm: String, day: LocalDate): Instant? {
        val m = Window.minutes(hm) ?: return null
        return ZonedDateTime.of(day, LocalTime.of(m / 60, m % 60), zone).toInstant()
    }

    private fun isWeekend(d: DayOfWeek) = d == DayOfWeek.SATURDAY || d == DayOfWeek.SUNDAY
    private fun isSchoolDay(d: DayOfWeek) = !isWeekend(d)
    private fun isWeekendNight(d: DayOfWeek) = d == DayOfWeek.FRIDAY || d == DayOfWeek.SATURDAY

    fun dayKind(s: Settings, now: Instant): DayKind =
        if (s.weekend.on && isWeekend(date(now).dayOfWeek)) DayKind.WEEKEND else DayKind.SCHOOL

    /** Today's Game time pool in minutes. */
    fun dailyMinutes(s: Settings, now: Instant): Int =
        if (dayKind(s, now) == DayKind.WEEKEND) s.weekend.minutes else s.gameMinutes

    fun bedtime(s: Settings, kind: DayKind): Window = if (kind == DayKind.WEEKEND) s.weekend.bedtime else s.bedtime

    /** The most recent night of [w] that starts at or before [now]. */
    fun lastNight(w: Window, now: Instant): Span? {
        val st = Window.minutes(w.start) ?: return null
        val en = Window.minutes(w.end) ?: return null
        if (st == en) return null
        val today = date(now)
        var startDay = today
        var start = clockTime(w.start, today) ?: return null
        if (start.isAfter(now)) {
            startDay = today.minusDays(1)
            start = clockTime(w.start, startDay) ?: return null
        }
        val endDay = if (en < st) startDay.plusDays(1) else startDay
        val end = clockTime(w.end, endDay) ?: return null
        return if (end.isAfter(start)) Span(start, end) else null
    }

    /** The day a night belongs to: the day it starts, or the evening before when it starts after midnight. */
    fun nightKind(s: Settings, nightStart: Instant): DayKind {
        if (!s.weekend.on) return DayKind.SCHOOL
        val local = nightStart.atZone(zone)
        val day = if (local.hour < 12) local.toLocalDate().minusDays(1) else local.toLocalDate()
        return if (isWeekendNight(day.dayOfWeek)) DayKind.WEEKEND else DayKind.SCHOOL
    }

    /** The bedtime in force at [now], or null. */
    fun bedtimeNow(s: Settings, now: Instant): Span? {
        if (!s.bedtimeOn) return null
        for (kind in if (s.weekend.on) DayKind.entries else listOf(DayKind.SCHOOL)) {
            val night = lastNight(bedtime(s, kind), now) ?: continue
            if (now in night && nightKind(s, night.start) == kind) return night
        }
        return null
    }

    /** Homework time in force at [now] (Monday to Friday, inside the window), or null. */
    fun homeworkNow(s: Settings, now: Instant): Span? {
        val h = s.homework
        if (!h.on || !validHomework(h.start, h.end)) return null
        val d = date(now)
        if (!isSchoolDay(d.dayOfWeek)) return null
        val span = Span(clockTime(h.start, d) ?: return null, clockTime(h.end, d) ?: return null)
        return if (now in span) span else null
    }

    /** The next homework start after [now] within a week, or null. */
    fun nextHomeworkStart(s: Settings, now: Instant): Instant? {
        val h = s.homework
        if (!h.on || !validHomework(h.start, h.end)) return null
        var d = date(now)
        repeat(8) {
            if (isSchoolDay(d.dayOfWeek)) clockTime(h.start, d)?.let { if (it.isAfter(now)) return it }
            d = d.plusDays(1)
        }
        return null
    }

    /** The next bedtime start after [now] that closes apps (its night kind matches), or null. */
    fun nextBedtimeStart(s: Settings, now: Instant): Instant? {
        if (!s.bedtimeOn) return null
        var best: Instant? = null
        for (kind in if (s.weekend.on) DayKind.entries else listOf(DayKind.SCHOOL)) {
            val w = bedtime(s, kind)
            var d = date(now)
            for (i in 0 until 9) {
                val st = clockTime(w.start, d)
                if (st != null && st.isAfter(now) && nightKind(s, st) == kind) {
                    if (best == null || st.isBefore(best)) best = st
                    break
                }
                d = d.plusDays(1)
            }
        }
        return best
    }

    fun pauseEnd(choice: PauseChoice, now: Instant): Instant = when (choice) {
        PauseChoice.MIN_30 -> now.plusSeconds(30 * 60)
        PauseChoice.HOUR_1 -> now.plusSeconds(60 * 60)
        PauseChoice.HOUR_2 -> now.plusSeconds(120 * 60)
        PauseChoice.UNTIL_6 -> {
            val six = clockTime("06:00", date(now))!!
            if (six.isAfter(now)) six else clockTime("06:00", date(now).plusDays(1))!!
        }
    }

    companion object {
        const val MIN_WINDOW_MINUTES = 15

        /** A bedtime of at least 15 minutes (it may cross midnight). */
        fun validNight(start: String, end: String): Boolean {
            val a = Window.minutes(start) ?: return false
            val b = Window.minutes(end) ?: return false
            return (b - a + 1440) % 1440 >= MIN_WINDOW_MINUTES
        }

        /** Homework time: same day, at least 15 minutes. */
        fun validHomework(start: String, end: String): Boolean {
            val a = Window.minutes(start) ?: return false
            val b = Window.minutes(end) ?: return false
            return b - a >= MIN_WINDOW_MINUTES
        }
    }
}
