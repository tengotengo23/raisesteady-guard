package com.raisesteady.guard.guard

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Which windows the PIN screen covers (design 3.6). */
class SettingsGuardTest {
    private val marks = SettingsGuard.Marks.FALLBACK
    private fun cover(pkg: String, title: String? = null, texts: List<String> = emptyList(), unlocked: Boolean = false) =
        SettingsGuard.shouldCover(pkg, title, texts, marks, unlocked)

    @Test fun ourPagesInSettingsAreCovered() {
        assertTrue(cover("com.android.settings", "App info", listOf("App info", "RaiseSteady Guard", "Force stop")))
        assertTrue(cover("com.google.android.packageinstaller", null, listOf("RaiseSteady Guard", "Do you want to uninstall this app?")))
        assertTrue(cover("com.android.permissioncontroller", null, listOf("RaiseSteady Guard")))
    }

    @Test fun otherAppsPagesAreNotCovered() {
        assertFalse(cover("com.android.settings", "App info", listOf("App info", "Chrome", "Force stop")))
        assertFalse(cover("com.android.settings", "Settings", listOf("Network and internet", "Connected devices")))
    }

    @Test fun guardedPagesAreFoundByTheirTitle() {
        assertTrue(cover("com.android.settings", "Private DNS"))
        assertTrue(cover("com.android.settings", "Date & time"))
        assertTrue(cover("com.android.settings", "date and time"))
        assertTrue(cover("com.android.settings", "Developer options"))
        assertTrue(cover("com.android.settings", "Дата и время"))
        // Android 14+ does not show the Device admin rows to us: the page is found by its title.
        assertTrue(cover("com.android.settings", "Device admin", listOf("Device admin", "Navigate up")))
        assertTrue(cover("com.android.settings", "Device admin apps"))
        // A row with that name on another page is not the page itself.
        assertFalse(cover("com.android.settings", "System", listOf("Date & time", "Developer options")))
    }

    @Test fun anyVpnConsentDialogIsCovered() {
        assertTrue(cover("com.android.vpndialogs", null, listOf("Connection request", "Other VPN")))
    }

    @Test fun onlyTheGuardedPackagesAreRead() {
        assertFalse(cover("com.whatsapp", "RaiseSteady Guard", listOf("RaiseSteady Guard")))
        assertFalse(cover("com.android.chrome", "Private DNS"))
    }

    @Test fun nothingIsCoveredWhileUnlocked() {
        assertFalse(cover("com.android.settings", "App info", listOf("RaiseSteady Guard"), unlocked = true))
        assertFalse(cover("com.android.vpndialogs", unlocked = true))
    }

    // Director's decision D1: our own accessibility page and its turn-off dialog are covered; the
    // Accessibility list never is (a child must be free to turn on TalkBack and the other aids).

    @Test fun theAccessibilityListIsNotCoveredThoughItShowsOurRow() {
        val list = listOf("Accessibility", "Screen readers", "TalkBack", "Downloaded apps", "RaiseSteady Guard", "On")
        assertFalse(cover("com.android.settings", "Accessibility", list))
        // A content change carries no title: the list is known by its heading on screen.
        assertFalse(cover("com.android.settings", null, listOf("Downloaded apps", "RaiseSteady Guard", "Off")))
        // Android 10: "Downloaded services"; Russian device.
        assertFalse(cover("com.android.settings", null, listOf("Downloaded services", "RaiseSteady Guard")))
        assertFalse(cover("com.android.settings", "Специальные возможности", listOf("RaiseSteady Guard")))
    }

    @Test fun talkBackAndOtherAidsAreNotCovered() {
        assertFalse(cover("com.android.settings", "TalkBack", listOf("TalkBack", "Use TalkBack", "Settings")))
        assertFalse(cover("com.android.settings", "Magnification", listOf("Magnification", "Magnification shortcut")))
        assertFalse(cover("com.android.settings", null, listOf("Stop TalkBack?", "Cancel", "Stop")))
    }

    @Test fun ourOwnAccessibilityPageAndItsTurnOffDialogAreCovered() {
        // The page is titled with our label (and its switch says "Use RaiseSteady Guard").
        assertTrue(cover("com.android.settings", "RaiseSteady Guard", listOf("RaiseSteady Guard", "Use RaiseSteady Guard")))
        // A stale list title with our page's switch on screen: still our page.
        assertTrue(cover("com.android.settings", "Accessibility", listOf("Use RaiseSteady Guard", "RaiseSteady Guard shortcut")))
        // The turn-off dialog over it (Android 10 and later wordings).
        assertTrue(cover("com.android.settings", null, listOf("Stop RaiseSteady Guard?", "Cancel", "OK")))
        assertTrue(cover("com.android.settings", null, listOf("Turn off RaiseSteady Guard?", "Cancel", "Turn off")))
    }

    @Test fun marksFromTheSettingsAppAreUsedAsGiven() {
        val ka = SettingsGuard.Marks(
            pages = emptySet(),
            a11yList = setOf("მარტივი წვდომა"),
            ourA11y = setOf("RaiseSteady Guard-ის გამოყენება"),
        )
        assertFalse(SettingsGuard.shouldCover("com.android.settings", "მარტივი წვდომა", listOf("RaiseSteady Guard"), ka, false))
        assertTrue(SettingsGuard.shouldCover("com.android.settings", "მარტივი წვდომა", listOf("RaiseSteady Guard-ის გამოყენება"), ka, false))
        // Two titles (the event's and the window's): either one can name the page.
        assertTrue(SettingsGuard.shouldCover("com.android.settings", listOf(null, "RaiseSteady Guard"), listOf("RaiseSteady Guard"), ka, false))
    }
}
