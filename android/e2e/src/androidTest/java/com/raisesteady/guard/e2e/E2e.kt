package com.raisesteady.guard.e2e

import android.os.ParcelFileDescriptor
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.rules.TestWatcher
import org.junit.runner.Description
import java.io.File
import java.net.InetAddress
import java.net.UnknownHostException

/** Shared helpers for the end-to-end tests (run from the test app, another UID than Guard). */
object E2e {
    const val GUARD = "com.raisesteady.guard"
    const val GAME = "com.raisesteady.guard.e2e"
    const val PIN = "2468"

    val instr get() = InstrumentationRegistry.getInstrumentation()
    val device: UiDevice get() = UiDevice.getInstance(instr)

    /** Runs a command as the shell user and returns what it printed. */
    fun shell(cmd: String): String {
        val pfd: ParcelFileDescriptor = instr.uiAutomation.executeShellCommand(cmd)
        return ParcelFileDescriptor.AutoCloseInputStream(pfd).use { it.reader().readText() }
    }

    fun resolves(host: String): Boolean = try {
        InetAddress.getAllByName(host).isNotEmpty()
    } catch (e: UnknownHostException) {
        false
    }

    /** Screenshots and window dumps; tools/e2e.sh pulls this folder into the CI artefact. */
    private val dir: File by lazy { File(instr.targetContext.getExternalFilesDir(null), "screens").apply { mkdirs() } }

    fun shot(name: String) {
        runCatching { device.takeScreenshot(File(dir, "$name.png")) }
        runCatching { File(dir, "$name.xml").outputStream().use { device.dumpWindowHierarchy(it) } }
    }

    /** Waits until an object with [text] (contained) is on screen. */
    fun waitText(text: String, ms: Long = 10_000): Boolean = device.wait(Until.hasObject(By.textContains(text)), ms) == true

    fun waitPackage(pkg: String, ms: Long = 10_000): Boolean = device.wait(Until.hasObject(By.pkg(pkg).depth(0)), ms) == true

    /** Taps the first object whose text contains [text]; false when none shows within [ms]. */
    fun tap(text: String, ms: Long = 10_000): Boolean {
        val o = device.wait(Until.findObject(By.textContains(text)), ms) ?: return false
        o.click()
        return true
    }

    /** Saves a screenshot and the window dump of a failing test. */
    class Shots : TestWatcher() {
        override fun failed(e: Throwable?, d: Description) = shot("${d.testClass.simpleName}-${d.methodName}-fail")
    }
}
