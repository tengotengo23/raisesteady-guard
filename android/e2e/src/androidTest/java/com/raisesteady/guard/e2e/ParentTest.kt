package com.raisesteady.guard.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Until
import com.raisesteady.guard.e2e.E2e.GUARD
import com.raisesteady.guard.e2e.E2e.byTag
import com.raisesteady.guard.e2e.E2e.device
import com.raisesteady.guard.e2e.E2e.shell
import com.raisesteady.guard.e2e.E2e.shot
import com.raisesteady.guard.e2e.E2e.waitText
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.FixMethodOrder
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.MethodSorters

/**
 * The parent session is short-lived (security review): leaving the app or recreating its screen
 * asks for the PIN again; a PIN change needs the current PIN. (Setup steps after setup and the
 * 5-minute idle close: RoutesTest, JVM.)
 */
@RunWith(AndroidJUnit4::class)
@FixMethodOrder(MethodSorters.NAME_ASCENDING)
class ParentTest {
    @get:Rule val shots = E2e.Shots()

    private val parentTop = "Pause game-time apps now"

    @Before fun guardOn() = E2e.enableGuard()

    @After fun home() {
        shell("settings put system font_scale 1.0")
        device.pressHome()
    }

    private fun openParentPage() {
        E2e.openGuard()
        assertTrue("no status page", E2e.tap("For the parent"))
        assertTrue(waitText("Enter the parent PIN", 5_000))
        E2e.typePin()
        assertTrue(E2e.tap("Next"))
        assertTrue("the parent page did not open", waitText(parentTop, 10_000))
    }

    /** Guard's app is in front again and shows the status page, not the parent page. */
    private fun backOnStatus(what: String) {
        assertTrue("$what: the status page did not show", waitText("For the parent", 10_000))
        assertFalse("$what: the parent page came back without the PIN", device.hasObject(By.text(parentTop)))
    }

    @Test fun a_backgroundThenBackAsksForThePin() {
        openParentPage()
        device.pressHome()
        Thread.sleep(1_500)
        // Back to the same task (from recents, as a child would): not the parent page.
        shell("am start -n $GUARD/.MainActivity")
        backOnStatus("after Home")
        shot("parent/after-background")
    }

    @Test fun b_recreatedScreenAsksForThePin() {
        openParentPage()
        // A configuration change recreates the activity (as a rotation does).
        shell("settings put system font_scale 1.15")
        Thread.sleep(2_500)
        backOnStatus("after the screen was recreated")
        shot("parent/after-recreate")
    }

    @Test fun c_pinChangeNeedsTheCurrentPin() {
        openParentPage()
        val before = Regex("\"hash\":\"([^\"]+)\"").find(E2e.guardSettingsJson())?.groupValues?.get(1)
        val field = E2e.scrollFind(byTag("pin-current"))
        assertTrue("no current PIN field", field != null)
        fun type(tag: String, v: String) {
            val o = E2e.scrollFind(byTag(tag)) ?: throw AssertionError("no $tag")
            (o.findObject(By.clazz("android.widget.EditText")) ?: o).text = v
        }
        type("pin-current", "1111")
        type("pin-new1", "9753")
        type("pin-new2", "9753")
        assertTrue(E2e.tapOn(byTag("pin-save"), scroll = true))
        assertTrue("a wrong current PIN was not refused", device.wait(Until.hasObject(By.text("Wrong PIN.")), 10_000) == true)
        shot("parent/pin-change-refused")
        val after = Regex("\"hash\":\"([^\"]+)\"").find(E2e.guardSettingsJson())?.groupValues?.get(1)
        assertEquals("the PIN changed without the current PIN", before, after)
        // The old PIN still opens the parent page.
        device.pressHome()
        openParentPage()
    }
}
