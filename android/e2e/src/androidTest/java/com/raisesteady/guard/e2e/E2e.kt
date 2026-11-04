package com.raisesteady.guard.e2e

import android.app.UiAutomation
import android.os.ParcelFileDescriptor
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Configurator
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

    const val A11Y = "$GUARD/$GUARD.guard.GuardAccessibilityService"
    const val SETTINGS = "com.android.settings"

    /** By default a test's UiAutomation switches other accessibility services off: Guard's must stay on. */
    private const val FLAGS = UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES

    init {
        Configurator.getInstance().uiAutomationFlags = FLAGS
    }

    val instr get() = InstrumentationRegistry.getInstrumentation()
    val device: UiDevice get() = UiDevice.getInstance(instr)

    /** Runs a command as the shell user (no shell syntax: words only) and returns what it printed. */
    fun shell(cmd: String): String {
        val pfd: ParcelFileDescriptor = instr.getUiAutomation(FLAGS).executeShellCommand(cmd)
        return ParcelFileDescriptor.AutoCloseInputStream(pfd).use { it.reader().readText() }
    }

    /** Guard's stored settings (debug build: run-as). */
    fun guardSettingsJson(): String = shell("run-as $GUARD cat files/datastore/settings.json")

    /**
     * Sets Guard up as a parent would before the setup screens exist (W6): age 9-12, PIN 2468,
     * nothing paused or unlocked, the filter on; then switches the accessibility service on and
     * waits until Android has bound it.
     */
    fun enableGuard(gameMinutes: Int? = null) {
        val extra = gameMinutes?.let { " --ei gameMinutes $it" } ?: ""
        shell("am start -n $GUARD/.debug.DebugFilterActivity --es band 9-12 --ez familyDns true --es pin $PIN --ez resetState true$extra")
        val want = gameMinutes?.let { "\"gameMinutes\":$it" } ?: "\"pin\":{"
        waitFor("Guard's settings were not written ($want)", 20_000) { guardSettingsJson().contains(want) }
        if (!shell("settings get secure enabled_accessibility_services").contains(A11Y)) {
            shell("settings put secure enabled_accessibility_services $A11Y")
            shell("settings put secure accessibility_enabled 1")
        }
        waitFor("Guard's accessibility service was not bound", 30_000) { guardStatus().contains("a11y=true") }
        Thread.sleep(1000)
    }

    /** What runs in Guard now (debug builds): "a11y=true vpn=true admin=false". */
    fun guardStatus(): String = shell("am broadcast -n $GUARD/.debug.DebugStatusReceiver")

    /** Moves Guard's clock (debug builds only) by [offsetMs]. */
    fun clockOffset(offsetMs: Long) {
        val out = shell("am broadcast -n $GUARD/.debug.DebugClockReceiver --el offsetMs $offsetMs")
        check(out.contains("offset $offsetMs")) { "clock offset not set: $out" }
    }

    fun waitFor(what: String, ms: Long, ok: () -> Boolean) {
        val end = System.currentTimeMillis() + ms
        while (!ok()) {
            if (System.currentTimeMillis() > end) throw AssertionError(what)
            Thread.sleep(300)
        }
    }

    /** Types into the first text field on screen (the PIN field). */
    fun typePin(pin: String = PIN) {
        val field = device.wait(Until.findObject(By.clazz("android.widget.EditText")), 5_000) ?: throw AssertionError("no PIN field")
        field.text = pin
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
