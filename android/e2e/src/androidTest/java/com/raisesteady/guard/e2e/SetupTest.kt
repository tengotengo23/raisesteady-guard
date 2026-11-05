package com.raisesteady.guard.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Until
import com.raisesteady.guard.e2e.E2e.GAME
import com.raisesteady.guard.e2e.E2e.GUARD
import com.raisesteady.guard.e2e.E2e.byTag
import com.raisesteady.guard.e2e.E2e.device
import com.raisesteady.guard.e2e.E2e.shell
import com.raisesteady.guard.e2e.E2e.shot
import com.raisesteady.guard.e2e.E2e.waitText
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Design 6.2 test 1: a parent walks the whole setup on a fresh install (tools/e2e.sh installs
 * fresh and runs this class first): age 9 to 12, PIN 2468, every permission step, the apps list
 * (the test game starts on Game time), the checks; the status page then says "Protection is on".
 * VPN consent and usage access are pre-approved by e2e.sh (as the parent's taps would);
 * accessibility and the device admin are switched on here by shell, as the parent would in
 * Settings. A screenshot per step.
 */
@RunWith(AndroidJUnit4::class)
class SetupTest {
    @get:Rule val shots = E2e.Shots()

    private fun title(text: String, ms: Long = 20_000) = assertTrue("no \"$text\" on screen", waitText(text, ms))

    /** Taps the step's action, or Next when its part is already on; then waits for the next step's title. */
    private fun advance(action: String, nextTitle: String) {
        val o = device.wait(Until.findObject(By.text(action)), 3_000) ?: device.wait(Until.findObject(By.text("Next")), 3_000)
        assertNotNull("neither \"$action\" nor Next", o)
        o!!.click()
        title(nextTitle)
    }

    /** Types into the text field with Compose test tag [tag]. */
    private fun type(tag: String, text: String) {
        val o = device.wait(Until.findObject(byTag(tag)), 5_000) ?: throw AssertionError("no field $tag")
        (o.findObject(By.clazz("android.widget.EditText")) ?: o).text = text
    }

    private fun step(n: Int) = assertTrue("not step $n", waitText("Step $n of 13", 5_000))

    @Test fun setupFromAFreshInstall() {
        // Fresh: as after a new install (e2e.sh installs fresh; this makes the class safe to run alone).
        // The release smoke has no debug hooks: the fresh install is all it needs.
        if (!E2e.release) {
            shell("am start -n $GUARD/.debug.DebugFilterActivity --ez fresh true")
            E2e.waitFor("settings were not reset", 10_000) { !E2e.guardSettingsJson().contains("\"setupDone\":true") }
        }
        E2e.openGuard()

        title("Make this device safer for your child")
        step(1)
        assertTrue(device.hasObject(By.textContains("Nothing is sent to us")))
        assertTrue(device.hasObject(By.textContains("Made with AI")))
        shot("setup/01-welcome")
        assertTrue(E2e.tap("Start"))

        title("Before you start")
        step(2)
        assertTrue("the terms are not shown", device.hasObject(By.textContains("NO GUARANTEES")))
        shot("setup/02-terms")
        assertTrue(E2e.tap("I agree"))

        title("Your child's age")
        step(3)
        device.findObject(byTag("band-9-12"))!!.click()
        shot("setup/03-age")
        assertTrue(E2e.tap("Next"))

        title("You need the PIN")
        step(4)
        type("pin1", E2e.PIN)
        type("pin2", E2e.PIN)
        shot("setup/04-pin")
        assertTrue(E2e.tap("Next"))

        title("Use the family DNS filter")
        step(5)
        shot("setup/05-dns")
        assertTrue(E2e.tap("Next"))

        title("Turn on the site filter")
        step(6)
        assertTrue(device.hasObject(By.textContains("sends nothing to us")))
        shot("setup/06-vpn")
        advance("Turn on the filter", "Allow notifications")

        step(7)
        shot("setup/07-notifications")
        advance("Allow", "Allow usage access")

        step(8)
        shot("setup/08-usage")
        advance("Open settings", "Let RaiseSteady Guard see which app is open")

        // Accessibility: its own disclosure screen with consent; then the parent switches it on in Settings.
        step(9)
        assertTrue(device.hasObject(By.textContains("It does not read messages, passwords or what is typed")))
        assertTrue(device.hasObject(By.text("I agree, open settings")))
        assertTrue(device.hasObject(By.text("Not now")))
        assertTrue(E2e.scrollFind(By.textContains("Allow restricted settings")) != null)
        shot("setup/09-accessibility")
        E2e.enableAccessibility()
        title("Stop the app being removed") // moves on by itself

        step(10)
        shot("setup/10-admin")
        shell("dpm set-active-admin $GUARD/.guard.GuardAdminReceiver")
        title("Keep the filter always on")

        step(11)
        shot("setup/11-always-on")
        assertTrue(E2e.tap("Skip for now"))

        title("Apps and games")
        step(12)
        type("app-search", "Test")
        val row = E2e.scrollFind(byTag("app-$GAME"))
        assertNotNull("the test game is not in the apps list", row)
        val game = row!!.findObject(byTag("level-GAME_TIME"))
        assertTrue("the test game does not start on Game time", game.isChecked || game.isSelected)
        shot("setup/12-apps")
        assertTrue(E2e.tap("Turn protection on"))

        title("Last checks")
        step(13)
        shot("setup/13-checks")
        assertTrue(E2e.tap("Done"))

        assertTrue("the status page does not say Protection is on", waitText("Protection is on", 20_000))
        shot("setup/14-status")
        if (E2e.release) { // run-as cannot read a release build's files
            device.pressHome()
            return
        }
        val stored = E2e.guardSettingsJson()
        assertTrue("setup not stored as done: $stored", stored.contains("\"setupDone\":true"))
        assertTrue("age band not stored", stored.contains("\"bandId\":\"9-12\""))
        assertTrue("PIN not stored", stored.contains("\"pin\":{"))
        device.pressHome()
    }
}
