package com.raisesteady.guard.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Until
import com.raisesteady.guard.e2e.E2e.GAME
import com.raisesteady.guard.e2e.E2e.byTag
import com.raisesteady.guard.e2e.E2e.device
import com.raisesteady.guard.e2e.E2e.shell
import com.raisesteady.guard.e2e.E2e.shot
import com.raisesteady.guard.e2e.E2e.waitText
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Design 6.2 test 5: after the test game was open for about two minutes, the parent page (behind
 * the PIN) shows it under Activity, Today, with at least 1 min. The report is written each minute
 * while a Game-time app is in front (usage.json, design 3.5).
 */
@RunWith(AndroidJUnit4::class)
class ReportTest {
    @get:Rule val shots = E2e.Shots()

    @Test fun todayShowsTheMinutesOfTheTestGame() {
        // No bedtime: CI runs at any hour (21:00 to 07:00 would close the game at once).
        E2e.enableGuard(gameMinutes = 240, bedtime = false)
        shell("am start -n $GAME/.TestGameActivity")
        assertTrue("the test game did not open", E2e.waitPackage(GAME, 10_000))
        // Open for 110 s: the first report line is written at once, the next one a minute or so later.
        assertFalse("the test game was closed", waitText("is closed for now", 110_000))
        device.pressHome()

        E2e.openGuard()
        assertTrue("no status page", waitText("For the parent", 10_000))
        assertTrue(E2e.tap("For the parent"))
        assertTrue(waitText("Enter the parent PIN", 5_000))
        E2e.typePin()
        assertTrue(E2e.tap("Next"))
        assertTrue("the parent page did not open", waitText("Pause game-time apps now", 10_000))
        shot("report/parent-top")

        val minutes = E2e.scrollFind(byTag("report-today-$GAME"))
        assertNotNull("Activity does not list the test game today", minutes)
        val text = minutes!!.text
        val n = Regex("(\\d+)").find(text)?.value?.toInt() ?: 0
        assertTrue("the test game shows less than 1 min today: \"$text\"", n >= 1)
        assertTrue("the test game's name is not shown", device.hasObject(By.text("Test game")))
        assertTrue(device.hasObject(By.text("Today")))
        assertNotNull(device.wait(Until.findObject(By.text("Last 7 days")), 3_000) ?: E2e.scrollFind(By.text("Last 7 days")))
        shot("report/activity")
        device.pressHome()
    }
}
