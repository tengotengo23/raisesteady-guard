package com.raisesteady.guard.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.raisesteady.guard.e2e.E2e.byTag
import com.raisesteady.guard.e2e.E2e.device
import com.raisesteady.guard.e2e.E2e.shell
import com.raisesteady.guard.e2e.E2e.shot
import com.raisesteady.guard.e2e.E2e.waitText
import androidx.test.uiautomator.Until
import org.junit.After
import org.junit.Assert.assertTrue
import org.junit.FixMethodOrder
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.MethodSorters

/**
 * Design 6.2 test 6. Every screen at font scale 1.0 and 2.0, light and dark: a screenshot each
 * (screens/look/<scale>-<theme>/, a CI artefact) and no text cut in its box (the debug look log
 * of GuardText). With "Remove animations" on, a screen still enters with the same 220 ms fade.
 */
@RunWith(AndroidJUnit4::class)
@FixMethodOrder(MethodSorters.NAME_ASCENDING)
class LookTest {
    @get:Rule val shots = E2e.Shots()

    @After fun back() {
        shell("settings put system font_scale 1.0")
        shell("cmd uimode night no")
        for (s in listOf("animator_duration_scale", "transition_animation_scale", "window_animation_scale")) shell("settings put global $s 1")
    }

    /** Opens each screen and saves it; [dir] is "1.0-light" and so on. */
    private fun everyScreen(dir: String) {
        fun take(name: String, open: () -> Unit, text: String) {
            open()
            assertTrue("$dir $name: \"$text\" did not show", waitText(text, 15_000))
            Thread.sleep(900) // the 220 ms entry, and the dark or large text settling
            shot("look/$dir/$name")
        }
        take("welcome", { E2e.openGuard("setup:1") }, "Make this device safer for your child")
        take("permission", { E2e.openGuard("setup:8") }, "Allow usage access")
        take("accessibility", { E2e.openGuard("setup:9") }, "Let RaiseSteady Guard see which app is open")
        take("apps", { E2e.openGuard("setup:12") }, "Apps and games")
        take("status", { E2e.openGuard("status") }, "Protection is")
        take("parent", { E2e.openGuard("parent") }, "Pause game-time apps now")
        take("block", { E2e.showChild("block", "BEDTIME") }, "Bedtime until")
        take("gate", { E2e.showChild("gate") }, "This page needs the parent PIN")
        device.pressHome()
    }

    @Test fun a_everyScreenAtTwoSizesInLightAndDark() {
        E2e.enableGuard()
        E2e.clearLook()
        for (scale in listOf("1.0", "2.0")) {
            for (dark in listOf(false, true)) {
                shell("settings put system font_scale $scale")
                shell("cmd uimode night ${if (dark) "yes" else "no"}")
                Thread.sleep(1_500)
                everyScreen("$scale-${if (dark) "dark" else "light"}")
            }
        }
        val log = E2e.look()
        assertTrue("texts were cut in their box: $log", log.contains("overflow=0"))
        // Each screen came in with its motion (8 screens x 4 looks).
        assertTrue("no screen entries in the look log: $log", log.contains("rise+fade 220 ms"))
    }

    @Test fun b_removeAnimationsGivesTheSameFade() {
        E2e.enableGuard()
        for (s in listOf("animator_duration_scale", "transition_animation_scale", "window_animation_scale")) shell("settings put global $s 0")
        Thread.sleep(1_000)
        E2e.clearLook()
        E2e.openGuard("status")
        assertTrue(waitText("Protection is", 15_000))
        Thread.sleep(1_500)
        val log = E2e.look()
        val motion = log.substringAfter("motion=")
        val m = Regex("(?:^|; )fade 220 ms \\(took (\\d+) ms\\)").find(motion)
        assertTrue("no 220 ms fade with animations removed: $log", m != null)
        assertTrue("the rise ran with animations removed: $log", !motion.contains("rise+fade"))
        // The fade really ran (not cut to 0 by the duration scale), within a frame or so of 220 ms or slower.
        assertTrue("the fade took ${m!!.groupValues[1]} ms", m.groupValues[1].toInt() >= 180)
        shot("look/no-animations-status")
        device.wait(Until.findObject(byTag("status-title")), 2_000)
        device.pressHome()
    }

    /** Review W8: the changed screens at font scale 2.0 (screens/w8/): status with its sub-line, parent Sites, block screen in Russian. */
    @Test fun c_changedScreensAtLargeText() {
        E2e.enableGuard()
        E2e.clearLook()
        shell("settings put system font_scale 2.0")
        Thread.sleep(1_500)
        E2e.openGuard("status")
        assertTrue(waitText("Protection is", 15_000))
        // Always-on VPN is off on the emulator: the title gets a sub-line.
        assertTrue("no \"to check\" sub-line under the status title", device.wait(Until.hasObject(byTag("status-to-check")), 5_000) == true)
        Thread.sleep(900)
        shot("w8/2.0-status")
        E2e.openGuard("parent")
        assertTrue(waitText("Pause game-time apps now", 15_000))
        assertTrue("no topic filter switch in Sites", E2e.scrollFind(byTag("topics-switch")) != null)
        Thread.sleep(600)
        shot("w8/2.0-parent-sites")
        device.pressHome()
        if (android.os.Build.VERSION.SDK_INT >= 33) {
            shell("cmd locale set-app-locales com.raisesteady.guard --locales ru")
            Thread.sleep(1_500)
            try {
                E2e.showChild("block", "PAUSED")
                assertTrue("the block screen is not in Russian", waitText("Приложение Test game сейчас закрыто", 15_000))
                assertTrue(device.hasObject(androidx.test.uiautomator.By.textContains("Родитель поставил на паузу до")))
                Thread.sleep(900)
                shot("w8/2.0-ru-block")
            } finally {
                shell("cmd locale set-app-locales com.raisesteady.guard --locales en")
                device.pressHome()
            }
        }
        val log = E2e.look()
        assertTrue("texts were cut in their box: $log", log.contains("overflow=0"))
    }
}
