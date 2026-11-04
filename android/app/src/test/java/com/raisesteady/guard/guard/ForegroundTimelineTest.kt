package com.raisesteady.guard.guard

import com.raisesteady.guard.rules.UsageMath
import org.junit.Assert.assertEquals
import org.junit.Test

/** The accessibility service's own record, used without usage access (design 3.3). */
class ForegroundTimelineTest {
    private val min = 60_000L

    @Test fun countsTheAppInFrontUntilTheNextOneOrScreenOff() {
        val t = ForegroundTimeline()
        t.moved("game", 0)
        t.moved("game", 2 * min) // the same app again: no new interval
        t.moved("notes", 5 * min)
        t.moved("game", 6 * min)
        t.screenOff(8 * min)
        val per = UsageMath.intervals(t.since(0), 10 * min).groupBy { it.pkg }.mapValues { e -> e.value.sumOf { it.lengthMs } }
        assertEquals(7 * min, per["game"])
        assertEquals(1 * min, per["notes"])
    }

    @Test fun anAppStillInFrontCountsToNow() {
        val t = ForegroundTimeline()
        t.moved("game", 1 * min)
        assertEquals(4 * min, UsageMath.intervals(t.since(0), 5 * min).single().lengthMs)
    }

    @Test fun anAppInFrontWhenTheDayBeganCountsFromTheStartOfTheDay() {
        val t = ForegroundTimeline()
        t.moved("game", 0)
        val day = 10 * min
        assertEquals(3 * min, UsageMath.intervals(t.since(day), 13 * min).single().lengthMs)
    }
}
