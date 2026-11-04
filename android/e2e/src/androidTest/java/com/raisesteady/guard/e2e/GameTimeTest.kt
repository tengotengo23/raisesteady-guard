package com.raisesteady.guard.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import com.raisesteady.guard.e2e.E2e.GAME
import com.raisesteady.guard.e2e.E2e.device
import com.raisesteady.guard.e2e.E2e.shell
import com.raisesteady.guard.e2e.E2e.shot
import com.raisesteady.guard.e2e.E2e.waitText
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.time.Duration
import java.time.Instant
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * Design 6.2 test 3: the test app is a game (android:appCategory="game"), so it starts on Game
 * time. With 1 minute a day (debug builds), Guard's block screen closes it when the minute is
 * used; the parent's PIN gives 15 more minutes and the game opens again; at bedtime (Guard's
 * clock moved to 21:01, debug builds only) it is closed again until 07:00.
 */
@RunWith(AndroidJUnit4::class)
class GameTimeTest {
    @get:Rule val shots = E2e.Shots()

    @After fun clockBack() = E2e.clockOffset(0)

    private val timeUp = "Today's game time is used up."

    private var offsetMs = 0L

    /** The clock offset that puts Guard's time at the first [at] after [after] (Guard time). */
    private fun offsetTo(after: Instant, at: LocalTime): Long {
        val zone = ZoneId.systemDefault()
        var t = ZonedDateTime.of(after.atZone(zone).toLocalDate(), at, zone)
        while (t.toInstant().isBefore(after)) t = t.plusDays(1)
        offsetMs = Duration.between(Instant.now(), t.toInstant()).toMillis()
        return offsetMs
    }

    private fun openGame() {
        shell("am start -W -n $GAME/.TestGameActivity")
        assertTrue("the test game did not open", E2e.waitPackage(GAME, 10_000))
    }

    @Test fun gameTimeMoreTimeAndBedtime() {
        E2e.enableGuard(gameMinutes = 1)
        // Guard's clock at the next 12:00 (outside bedtime: CI runs at any hour).
        E2e.clockOffset(offsetTo(Instant.now(), LocalTime.NOON))
        openGame()
        shot("game-open")
        // Not closed at once (earlier tests used the test app for a few seconds at most) ...
        assertFalse("the game was closed at once", waitText(timeUp, 10_000))
        // ... but when the minute is used (usage events can come late: the guard looks again every 30 s at most).
        assertTrue("the block screen did not come when the minute was used", waitText(timeUp, 100_000))
        assertTrue(device.hasObject(By.textContains("is closed for now")))
        shot("block-time-up")

        // The parent gives 15 more minutes with the PIN: the game opens again and stays open.
        assertTrue("no For the parent button", E2e.tap("For the parent"))
        E2e.typePin()
        assertTrue("no 15 more minutes button", E2e.tap("15 more minutes"))
        assertTrue("the game did not open again", E2e.waitPackage(GAME, 15_000))
        assertFalse("the game was closed again during its more time", waitText("is closed for now", 8_000))
        shot("more-time")

        // Bedtime: Guard's clock at the next 21:01 after the 15 more minutes (band 9-12: 21:00 to 07:00).
        E2e.clockOffset(offsetTo(Instant.now().plusMillis(offsetMs).plus(Duration.ofMinutes(20)), LocalTime.of(21, 1)))
        if (!E2e.waitPackage(GAME, 1_000)) openGame()
        assertTrue("the block screen did not come at bedtime", waitText("Bedtime until", 30_000))
        assertTrue("the bedtime does not end at 7:00", device.hasObject(By.textContains("7:00")))
        shot("block-bedtime")
    }
}
