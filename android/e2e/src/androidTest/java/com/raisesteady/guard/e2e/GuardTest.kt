package com.raisesteady.guard.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import com.raisesteady.guard.e2e.E2e.GUARD
import com.raisesteady.guard.e2e.E2e.SETTINGS
import com.raisesteady.guard.e2e.E2e.device
import com.raisesteady.guard.e2e.E2e.shell
import com.raisesteady.guard.e2e.E2e.shot
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.FixMethodOrder
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.MethodSorters
import java.util.regex.Pattern

/**
 * Design 6.2 test 4: Guard's own pages in Settings are covered by the PIN screen within 2 s of
 * opening; the right PIN opens them for 10 minutes and "Lock now" (on the notice, and on the
 * parent page: D2) closes them again; while Guard is a device admin, uninstalling it is refused.
 * The Accessibility list stays open, TalkBack and the other aids too; only Guard's own page there
 * is covered (D1). Tests run in name order (uninstall last).
 */
@RunWith(AndroidJUnit4::class)
@FixMethodOrder(MethodSorters.NAME_ASCENDING)
class GuardTest {
    @get:Rule val shots = E2e.Shots()

    private val gate = "This page needs the parent PIN"
    private val forceStop = By.text(Pattern.compile("(?i)force stop"))

    @Before fun guardOn() {
        E2e.enableGuard()
        shell("dpm set-active-admin $GUARD/.guard.GuardAdminReceiver")
        E2e.device.pressHome()
    }

    /** Opens a Settings page by [cmd]; returns true when the PIN screen covered it within 2 s of Settings showing. */
    private fun covered(cmd: String, what: String): Boolean {
        shell("am force-stop $SETTINGS") // each page opens fresh, not over a page left from before
        val out = shell(cmd)
        assertFalse("$what did not start: $out", out.contains("Error"))
        val end = System.currentTimeMillis() + 15_000
        var seen = 0L
        while (System.currentTimeMillis() < end) {
            if (device.hasObject(By.text(gate))) return true
            if (seen == 0L && device.hasObject(By.pkg(SETTINGS).depth(0))) seen = System.currentTimeMillis()
            if (seen != 0L && System.currentTimeMillis() - seen > 2_000) break
            Thread.sleep(100)
        }
        shot("not-covered-${what.replace(' ', '-')}")
        return false
    }

    private val appInfo = "am start -a android.settings.APPLICATION_DETAILS_SETTINGS -d package:$GUARD"

    @Test fun a_appInfoIsCoveredAndBackGoesHome() {
        assertTrue("App info was not covered", covered(appInfo, "App info"))
        assertFalse("Force stop can be tapped", device.hasObject(forceStop))
        shot("cover-app-info")
        assertTrue(E2e.tap("Back"))
        assertTrue("Back did not leave Settings", device.wait(androidx.test.uiautomator.Until.gone(By.text(gate)), 5_000) == true)
        assertFalse("Back went to Settings", device.hasObject(By.pkg(SETTINGS).depth(0)))
    }

    @Test fun b_vpnAndDeviceAdminPagesAreCovered() {
        assertTrue("VPN settings were not covered", covered("am start -a android.settings.VPN_SETTINGS", "VPN settings"))
        device.pressHome()
        assertTrue(
            "Device admin apps were not covered",
            covered("am start -n $SETTINGS/.Settings\$DeviceAdminSettingsActivity", "Device admin apps"),
        )
        device.pressHome()
    }

    /** D1: the Accessibility list and the other aids stay open; Guard's own page in it is covered. */
    @Test fun ba_accessibilityListStaysOpenButOurPageIsCovered() {
        shell("am force-stop $SETTINGS")
        shell("am start -a android.settings.ACCESSIBILITY_SETTINGS")
        assertTrue("Accessibility settings did not open", E2e.waitPackage(SETTINGS, 10_000))
        assertTrue("the Accessibility list was covered", E2e.staysAway(gate, 4_000))
        shot("a11y-list-open")

        // Another aid (TalkBack where the image has it, or Magnification, in every Android) opens freely.
        val aidName = listOf("TalkBack", "Magnification").firstOrNull { E2e.scrollFind(By.text(it), 6_000) != null }
        assertTrue("no TalkBack or Magnification row in Accessibility", aidName != null)
        assertTrue("could not open $aidName", E2e.tapOn(By.text(aidName!!), scroll = true))
        assertTrue("$aidName was covered", E2e.staysAway(gate, 4_000))
        assertTrue("$aidName did not open in Settings", device.hasObject(By.pkg(SETTINGS).depth(0)))
        shot("a11y-aid-open")

        // Guard's own page in the list is covered.
        shell("am force-stop $SETTINGS")
        shell("am start -a android.settings.ACCESSIBILITY_SETTINGS")
        assertTrue(E2e.waitPackage(SETTINGS, 10_000))
        assertTrue("Guard's row is not in the Accessibility list", E2e.tapOn(By.text("RaiseSteady Guard"), 20_000, scroll = true))
        assertTrue("Guard's own accessibility page was not covered", E2e.waitText(gate, 4_000))
        shot("a11y-our-page-covered")
        device.pressHome()
    }

    @Test fun c_thePinOpensSettingsAndLockNowClosesThem() {
        assertTrue(covered(appInfo, "App info"))
        E2e.typePin()
        assertTrue(E2e.tap("Unlock for 10 minutes"))
        assertTrue("App info did not show after the PIN", device.wait(androidx.test.uiautomator.Until.hasObject(forceStop), 10_000) == true)
        assertFalse("covered again while unlocked", E2e.waitText(gate, 3_000))
        shot("unlocked-app-info")
        device.pressHome()

        // "Lock now" on the notice. Android may fold it into a group: open the group, then the notice.
        device.openNotification()
        val notice = By.textContains("Settings are unlocked")
        assertTrue("no unlocked notice", device.wait(androidx.test.uiautomator.Until.hasObject(notice), 5_000) == true)
        val lockNow = Pattern.compile("(?i)lock now")
        fun findLock() = device.findObject(By.text(lockNow)) ?: device.findObject(By.desc(lockNow))
        val ours = By.res("com.android.systemui", "expandableNotificationRow").hasDescendant(notice)
        val expand = By.res("android", "expand_button").desc(Pattern.compile("(?i)expand.*"))
        repeat(4) {
            if (findLock() != null) return@repeat
            device.findObjects(ours).asReversed().firstNotNullOfOrNull { it.findObject(expand) }?.click()
            Thread.sleep(1500)
        }
        val lock = findLock()
        shot("notice-unlocked")
        assertTrue("no Lock now on the notice", lock != null)
        lock!!.click()
        device.pressHome()
        Thread.sleep(1000)
        assertTrue("App info was not covered after Lock now", covered(appInfo, "App info after Lock now"))
        device.pressHome()
    }

    /** D2: "Lock now" on the parent page closes the unlocked Settings pages too. */
    @Test fun d_lockNowOnTheParentPage() {
        assertTrue(covered(appInfo, "App info"))
        E2e.typePin()
        assertTrue(E2e.tap("Unlock for 10 minutes"))
        assertTrue("App info did not show after the PIN", device.wait(androidx.test.uiautomator.Until.hasObject(forceStop), 10_000) == true)
        device.pressHome()

        E2e.openGuard()
        assertTrue(E2e.tap("For the parent"))
        assertTrue(E2e.waitText("Enter the parent PIN", 5_000))
        E2e.typePin()
        assertTrue(E2e.tap("Next"))
        assertTrue("the parent page did not open", E2e.waitText("Pause game-time apps now", 10_000))
        val lock = E2e.scrollFind(E2e.byTag("lock-now"))
        assertTrue("no Lock now on the parent page while Settings are unlocked", lock != null)
        assertTrue(device.hasObject(By.textContains("Settings are unlocked until")))
        shot("parent-lock-now")
        assertTrue(E2e.tapOn(E2e.byTag("lock-now"), scroll = true))
        assertTrue("the parent page does not say the pages are locked", E2e.waitText("Settings pages need the parent PIN", 5_000))
        device.pressHome()
        Thread.sleep(1000)
        assertTrue("App info was not covered after Lock now on the parent page", covered(appInfo, "App info after parent Lock now"))
        device.pressHome()
    }

    @Test fun e_uninstallIsRefusedWhileDeviceAdmin() {
        val admins = shell("dumpsys device_policy")
        assertTrue("Guard is not an active device admin", admins.contains("$GUARD/.guard.GuardAdminReceiver"))
        val out = shell("pm uninstall $GUARD")
        assertTrue("uninstall was not refused: $out", out.contains("DELETE_FAILED_DEVICE_POLICY_MANAGER"))
        assertTrue("Guard is gone", shell("pm path $GUARD").contains("package:"))
    }
}
