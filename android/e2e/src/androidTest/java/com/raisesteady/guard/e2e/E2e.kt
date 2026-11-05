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
     * Sets Guard up quickly (SetupTest walks the real setup screens once): age 9-12, PIN 2468,
     * setup done, nothing paused or unlocked, the filter on; then switches the accessibility
     * service on and waits until Android has bound it.
     */
    fun enableGuard(gameMinutes: Int? = null) {
        val extra = gameMinutes?.let { " --ei gameMinutes $it" } ?: ""
        shell("am start -n $GUARD/.debug.DebugFilterActivity --es band 9-12 --ez familyDns true --es pin $PIN --ez resetState true$extra")
        val want = gameMinutes?.let { "\"gameMinutes\":$it" } ?: "\"pin\":{"
        waitFor("Guard's settings were not written ($want)", 20_000) { guardSettingsJson().let { it.contains(want) && it.contains("\"setupDone\":true") } }
        enableAccessibility()
        Thread.sleep(1000)
    }

    /** What runs in Guard now (debug builds): "a11y=true vpn=true admin=false". */
    fun guardStatus(): String = shell("am broadcast -n $GUARD/.debug.DebugStatusReceiver")

    /** Moves Guard's clock (debug builds only) by [offsetMs]. */
    fun clockOffset(offsetMs: Long) {
        val out = shell("am broadcast -n $GUARD/.debug.DebugClockReceiver --el offsetMs $offsetMs")
        check(out.contains("offset $offsetMs")) { "clock offset not set: $out" }
    }

    /** Switches Guard's accessibility service on (as a parent does in Settings) and waits until it is bound. */
    fun enableAccessibility() {
        if (!shell("settings get secure enabled_accessibility_services").contains(A11Y)) {
            shell("settings put secure enabled_accessibility_services $A11Y")
            shell("settings put secure accessibility_enabled 1")
        }
        waitFor("Guard's accessibility service was not bound", 30_000) { guardStatus().contains("a11y=true") }
    }

    /** Opens Guard's app at [route] ("setup:8", "status", "parent", "apps"; debug builds), in a new task. */
    fun openGuard(route: String? = null) {
        val extra = route?.let { " --es debugScreen $it" } ?: ""
        shell("am start -n $GUARD/.MainActivity -f 0x10008000$extra")
    }

    /** Opens a child screen (debug builds): "block" with a reason, or "gate". */
    fun showChild(screen: String, reason: String = "TIME_UP") {
        shell("am start -n $GUARD/.debug.DebugShowActivity --es screen $screen --es reason $reason")
    }

    /** The look log (debug builds): texts that overflowed their box and the screen entries' motion. */
    fun look(clear: Boolean = false): String =
        shell("am broadcast -n $GUARD/.debug.DebugStatusReceiver --ez look true --ez clearLook $clear")

    fun clearLook() {
        shell("am broadcast -n $GUARD/.debug.DebugStatusReceiver --ez clearLook true")
    }

    /** An object by its Compose test tag (Guard's screens publish tags as resource ids). */
    fun byTag(tag: String) = By.res(tag)

    /**
     * Finds [selector] on screen, scrolling the page down (and then up) when needed: Guard's pages
     * and Settings lists are longer than the screen at large text.
     */
    fun scrollFind(selector: androidx.test.uiautomator.BySelector, ms: Long = 10_000): androidx.test.uiautomator.UiObject2? {
        device.wait(Until.findObject(selector), 2_000)?.let { return it }
        val end = System.currentTimeMillis() + ms
        while (System.currentTimeMillis() < end) {
            val list = device.findObjects(By.scrollable(true)).maxByOrNull { it.visibleBounds.height() }
            if (list == null) {
                device.wait(Until.findObject(selector), 1_000)?.let { return it }
                continue
            }
            for (dir in listOf(androidx.test.uiautomator.Direction.DOWN, androidx.test.uiautomator.Direction.UP)) {
                repeat(25) {
                    device.findObject(selector)?.let { return it }
                    if (!list.scroll(dir, 0.6f)) return@repeat
                }
            }
            device.findObject(selector)?.let { return it }
        }
        return device.findObject(selector)
    }

    /** True when [text] does not show for [ms] (nothing covered the page). */
    fun staysAway(text: String, ms: Long): Boolean {
        val end = System.currentTimeMillis() + ms
        while (System.currentTimeMillis() < end) {
            if (device.hasObject(By.text(text))) return false
            Thread.sleep(150)
        }
        return true
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

    /** [name] may hold folders ("look/2.0-dark/status"). */
    fun shot(name: String) {
        File(dir, name).parentFile?.mkdirs()
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
