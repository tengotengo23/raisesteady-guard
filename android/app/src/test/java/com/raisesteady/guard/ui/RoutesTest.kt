package com.raisesteady.guard.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** The parent session is short-lived and setup is closed once done (security review of MainActivity). */
class RoutesTest {
    @Test fun theParentPageComesOnlyAfterThePinScreen() {
        assertEquals("parent", Routes.next("pin", "parent", setupDone = true))
        assertEquals("pin", Routes.next("status", "parent", setupDone = true))
        assertEquals("pin", Routes.next("status", "apps", setupDone = true))
        assertEquals("apps", Routes.next("parent", "apps", setupDone = true))
        assertEquals("parent", Routes.next("apps", "parent", setupDone = true))
        assertEquals("status", Routes.next("status", "off", setupDone = true))
    }

    @Test fun aRecreatedOrRestoredActivityNeverOpensBehindThePin() {
        for (r in listOf("parent", "apps", "off")) assertEquals(r, "status", Routes.restored(r, setupDone = true))
        assertEquals("setup:1", Routes.restored("parent", setupDone = false))
        assertEquals("status", Routes.restored(null, setupDone = true))
        assertEquals("setup:4", Routes.restored("setup:4", setupDone = false))
    }

    @Test fun leavingTheAppEndsTheParentSession() {
        assertEquals("status", Routes.onLeave("parent", setupDone = true))
        assertEquals("status", Routes.onLeave("apps", setupDone = true))
        assertEquals("status", Routes.onLeave("pin", setupDone = true))
        assertEquals("setup:6", Routes.onLeave("setup:6", setupDone = false)) // the parent comes back from Settings mid-setup
    }

    @Test fun setupStepsAreRefusedOnceSetupIsDone() {
        assertEquals("status", Routes.next("status", "setup:1", setupDone = true))
        assertEquals("status", Routes.next("setup:13", "setup:12", setupDone = true)) // Back from the last step
        assertEquals("status", Routes.next("parent", "setup:3", setupDone = true))
        assertEquals("status", Routes.restored("setup:5", setupDone = true))
        // Only "Turn protection on" on step 12 leads on to the checks, step 13.
        assertEquals("setup:13", Routes.next("setup:12", "setup:13", setupDone = true))
        assertEquals("setup:3", Routes.next("setup:2", "setup:3", setupDone = false))
    }

    @Test fun theParentPageClosesAfterFiveMinutesWithoutATouch() {
        assertEquals(5 * 60_000L, Routes.IDLE_MS)
        assertFalse(Routes.idle("parent", 0, Routes.IDLE_MS - 1))
        assertTrue(Routes.idle("parent", 0, Routes.IDLE_MS))
        assertTrue(Routes.idle("apps", 0, Routes.IDLE_MS + 1))
        assertFalse(Routes.idle("status", 0, 10 * Routes.IDLE_MS))
    }
}
