package com.raisesteady.guard.pin

import com.raisesteady.guard.data.PinLock
import com.raisesteady.guard.data.Repo
import com.raisesteady.guard.data.Settings
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class PinTest {
    @get:Rule val tmp = TemporaryFolder()

    private val rec by lazy { Pin.hash("2468") }

    @Test fun hashThenVerify() {
        assertEquals("pbkdf2-sha256", rec.alg)
        assertEquals(150_000, rec.rounds)
        assertTrue(Pin.matches("2468", rec))
        assertNotEquals(Pin.hash("2468").salt, rec.salt) // a new salt every time
    }

    @Test fun wrongPin() {
        assertFalse(Pin.matches("2469", rec))
        assertFalse(Pin.matches("", rec))
        assertFalse(Pin.matches("2468", null))
    }

    @Test fun fourToEightDigitsOnly() {
        for (ok in listOf("1234", "12345678")) assertTrue(ok, Pin.valid(ok))
        for (bad in listOf("123", "123456789", "12a4", " 1234", "١٢٣٤")) assertFalse(bad, Pin.valid(bad))
    }

    @Test fun fiveWrongTriesLockFor15Minutes() {
        var lock = PinLock()
        val t0 = 1_000_000L
        repeat(4) {
            val (r, next) = Pin.check("0000", rec, lock, t0)
            assertFalse(r.ok)
            assertFalse(r.locked)
            lock = next
        }
        val (fifth, locked) = Pin.check("0000", rec, lock, t0)
        assertTrue(fifth.locked)
        assertEquals(t0 + 15 * 60_000, fifth.until)
        // Even the right PIN is refused while locked.
        val (during, _) = Pin.check("2468", rec, locked, t0 + 14 * 60_000)
        assertFalse(during.ok)
        assertTrue(during.locked)
        // After 15 minutes the right PIN works and the count starts again.
        val (after, cleared) = Pin.check("2468", rec, locked, t0 + 15 * 60_000)
        assertTrue(after.ok)
        assertEquals(PinLock(), cleared)
    }

    @Test fun theLockSurvivesARestart() = runBlocking {
        val dir = tmp.newFolder("datastore")
        val scope1 = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        val repo1 = Repo(dir, scope1)
        repo1.updateSettings { Settings().copy(pin = rec) }
        repo1.updateState { it.copy(pinLock = PinLock(fails = 0, until = 5_000_000L)) }
        scope1.coroutineContext[kotlinx.coroutines.Job]!!.cancelAndJoin()
        // A new process reads the same files.
        val scope2 = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        val repo2 = Repo(dir, scope2)
        val lock = repo2.readState().pinLock
        val (r, _) = Pin.check("2468", repo2.readSettings().pin, lock, 4_000_000L)
        assertTrue(r.locked)
        assertTrue(Pin.matches("2468", repo2.readSettings().pin))
        scope2.coroutineContext[kotlinx.coroutines.Job]!!.cancelAndJoin()
    }

    @Test fun pbkdf2TakesUnderASecond() {
        val stored = rec // hashed before the clock starts: one PIN check is one derivation
        val t0 = System.nanoTime()
        assertTrue(Pin.matches("2468", stored))
        val ms = (System.nanoTime() - t0) / 1_000_000
        println("PBKDF2-SHA256 150,000 rounds: $ms ms on the JVM")
        assertTrue("took $ms ms", ms < 1000)
    }
}
