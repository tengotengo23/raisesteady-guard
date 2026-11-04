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
 * opening; the right PIN opens them for 10 minutes and "Lock now" closes them again; while Guard
 * is a device admin, uninstalling it is refused. Tests run in name order (uninstall last).
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

    @Test fun b_vpnAccessibilityAndDeviceAdminPagesAreCovered() {
        assertTrue("VPN settings were not covered", covered("am start -a android.settings.VPN_SETTINGS", "VPN settings"))
        device.pressHome()
        assertTrue("Accessibility settings were not covered", covered("am start -a android.settings.ACCESSIBILITY_SETTINGS", "Accessibility"))
        device.pressHome()
        assertTrue(
            "Device admin apps were not covered",
            covered("am start -n $SETTINGS/.Settings\$DeviceAdminSettingsActivity", "Device admin apps"),
        )
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

        // "Lock now" on the notice: the page needs the PIN again.
        device.openNotification()
        val lockNow = By.text(Pattern.compile("(?i)lock now"))
        val ours = By.res("com.android.systemui", "expandableNotificationRow").hasDescendant(By.textContains("Settings are unlocked"))
        var lock = device.wait(androidx.test.uiautomator.Until.findObject(lockNow), 3_000)
        repeat(3) {
            if (lock != null) return@repeat
            // Collapsed (or folded into a group): expand the innermost row that holds our notice.
            device.findObjects(ours).asReversed().firstNotNullOfOrNull { row ->
                row.findObject(By.res("android", "expand_button")) ?: row.findObject(By.res("android", "alternate_expand_target"))
            }?.click()
            lock = device.wait(androidx.test.uiautomator.Until.findObject(lockNow), 3_000)
        }
        shot("notice-unlocked")
        assertTrue("no Lock now on the notice", lock != null)
        lock!!.click()
        device.pressHome()
        Thread.sleep(1000)
        assertTrue("App info was not covered after Lock now", covered(appInfo, "App info after Lock now"))
        device.pressHome()
    }

    @Test fun d_uninstallIsRefusedWhileDeviceAdmin() {
        val admins = shell("dumpsys device_policy")
        assertTrue("Guard is not an active device admin", admins.contains("$GUARD/.guard.GuardAdminReceiver"))
        val out = shell("pm uninstall $GUARD")
        assertTrue("uninstall was not refused: $out", out.contains("DELETE_FAILED_DEVICE_POLICY_MANAGER"))
        assertTrue("Guard is gone", shell("pm path $GUARD").contains("package:"))
    }
}
