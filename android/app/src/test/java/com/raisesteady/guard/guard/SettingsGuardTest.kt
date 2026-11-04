package com.raisesteady.guard.guard

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Which windows the PIN screen covers (design 3.6). */
class SettingsGuardTest {
    private val titles = SettingsGuard.FALLBACK_TITLES
    private fun cover(pkg: String, title: String? = null, texts: List<String> = emptyList(), unlocked: Boolean = false) =
        SettingsGuard.shouldCover(pkg, title, texts, titles, unlocked)

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
}
