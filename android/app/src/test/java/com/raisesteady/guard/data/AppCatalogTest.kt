package com.raisesteady.guard.data

import com.raisesteady.guard.TestData
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Test

/** Default app levels from the real android-apps.json (design 2.4). */
class AppCatalogTest {
    private val catalog = AppCatalog.parse(TestData.asset("android-apps.json").readText())
    private fun level(pkg: String, ageMin: Int, isGame: Boolean = false) = catalog.defaultLevel(pkg, isGame, ageMin)

    @Test fun gamesAndVideoStartOnGameTime() {
        for (age in listOf(3, 9, 16)) {
            assertEquals(Level.GAME_TIME, level("com.roblox.client", age))
            assertEquals(Level.GAME_TIME, level("com.mojang.minecraftpe", age))
            assertEquals(Level.GAME_TIME, level("com.google.android.youtube", age))
        }
    }

    @Test fun anyAppInTheGameCategoryStartsOnGameTime() {
        assertEquals(Level.GAME_TIME, level("com.example.somegame", 9, isGame = true))
        assertEquals(Level.ALLOWED, level("com.example.notes", 9))
    }

    @Test fun youTubeKidsStaysAllowed() {
        assertEquals(Level.ALLOWED, level("com.google.android.apps.youtube.kids", 3, isGame = true))
    }

    @Test fun socialIsBlockedUnderItsMinimumAgeThenGameTime() {
        assertEquals(Level.BLOCKED, level("com.zhiliaoapp.musically", 9))
        assertEquals(Level.GAME_TIME, level("com.zhiliaoapp.musically", 13))
        assertEquals(Level.BLOCKED, level("com.instagram.android", 9))
        // VK and OK are 14: blocked for the 13 to 15 band (its lowest age is 13).
        assertEquals(Level.BLOCKED, level("com.vkontakte.android", 13))
        assertEquals(Level.GAME_TIME, level("com.vkontakte.android", 16))
    }

    @Test fun chatIsBlockedUnderItsMinimumAgeThenAllowed() {
        assertEquals(Level.BLOCKED, level("com.whatsapp", 9))
        assertEquals(Level.ALLOWED, level("com.whatsapp", 13))
        assertEquals(Level.BLOCKED, level("org.telegram.messenger", 16))
    }

    @Test fun bypassAppsAreBlockedAtEveryAge() {
        for (age in listOf(3, 9, 13, 16)) {
            assertEquals(Level.BLOCKED, level("org.torproject.torbrowser", age))
            assertEquals(Level.BLOCKED, level("com.psiphon3.subscription", age))
        }
    }

    @Test fun theParentsChoiceWins() {
        val s = Settings(appLevels = mapOf("com.roblox.client" to Level.ALLOWED, "com.whatsapp" to Level.BLOCKED))
        assertEquals(Level.ALLOWED, catalog.level("com.roblox.client", true, s, 9))
        assertEquals(Level.BLOCKED, catalog.level("com.whatsapp", false, s, 16))
        assertEquals(Level.GAME_TIME, catalog.level("com.mojang.minecraftpe", false, s, 9))
    }

    @Test fun everyPackageHasAnEntry() {
        assertNotNull(catalog.entry("com.ss.android.ugc.trill"))
        assertNotNull(catalog.entry("com.facebook.lite"))
    }
}
