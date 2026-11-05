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

    // Director's decision D3: the record is saved each minute and survives the process dying.

    @Test fun aSavedRecordCountsTheSameAfterARestart() {
        val t = ForegroundTimeline()
        t.moved("game", 1 * min)
        t.moved("notes", 4 * min)
        t.moved("game", 5 * min)
        val rec = t.record(0, 7 * min) // saved at 7 min, then the process dies
        val json = kotlinx.serialization.json.Json.encodeToString(TimelineRecord.serializer(), rec)
        val back = kotlinx.serialization.json.Json.decodeFromString(TimelineRecord.serializer(), json)

        val restarted = ForegroundTimeline()
        restarted.restore(back, 0)
        val per = UsageMath.intervals(restarted.since(0), 20 * min).groupBy { it.pkg }.mapValues { e -> e.value.sumOf { it.lengthMs } }
        // The game counts up to the save (nothing is known after it), not to "now".
        assertEquals(5 * min, per["game"])
        assertEquals(1 * min, per["notes"])
    }

    @Test fun eventsAfterTheRestartAddToTheSavedOnes() {
        val t = ForegroundTimeline()
        t.moved("game", 0)
        val rec = t.record(0, 3 * min)
        val restarted = ForegroundTimeline()
        restarted.moved("game", 10 * min) // the service saw the game again after it started
        restarted.restore(rec, 0)
        val game = UsageMath.intervals(restarted.since(0), 12 * min).filter { it.pkg == "game" }.sumOf { it.lengthMs }
        assertEquals(5 * min, game)
    }

    @Test fun aRecordFromAnEarlierDayIsNotRestored() {
        val t = ForegroundTimeline()
        t.moved("game", 0)
        val rec = t.record(0, 3 * min)
        val restarted = ForegroundTimeline()
        restarted.restore(rec, 60 * min) // today began after the save
        assertEquals(emptyList<Any>(), UsageMath.intervals(restarted.since(60 * min), 70 * min))
    }
}
