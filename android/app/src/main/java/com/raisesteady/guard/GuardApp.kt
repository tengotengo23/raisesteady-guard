package com.raisesteady.guard

import android.app.Application
import com.raisesteady.guard.data.AgeBands
import com.raisesteady.guard.data.AppCatalog
import com.raisesteady.guard.data.Repo
import com.raisesteady.guard.dns.Decision
import com.raisesteady.guard.dns.FilterSettings
import com.raisesteady.guard.lists.GuardData
import com.raisesteady.guard.pin.Clock
import com.raisesteady.guard.pin.DeviceClock
import com.raisesteady.guard.vpn.Notices
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong

/** The application: the store, shared data, the filter's settings and the notification channels. */
class GuardApp : Application() {
    /** Lives as long as the process. */
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    /** The block lists and helplines, loaded once from the assets (about 1.4 MB). */
    val data: GuardData by lazy { GuardData.load { assets.open(it) } }

    val bands: AgeBands by lazy { AgeBands.parse(assets.open("age-bands.json").use { it.reader().readText() }) }

    lateinit var repo: Repo
        private set

    /** Default levels of known Android apps (shared/android-apps.json). */
    val catalog: AppCatalog by lazy { AppCatalog.parse(assets.open("android-apps.json").use { it.reader().readText() }) }

    /** Debug builds only: a test offset for the clock (debug/DebugClockReceiver); always 0 in release builds. */
    val debugClockOffsetMs = AtomicLong(0)

    /** The time Guard uses. Release builds use the device clock as it is. */
    val clock: Clock = if (BuildConfig.DEBUG) DeviceClock { debugClockOffsetMs.get() } else DeviceClock()

    /** Asks the guard to check the app on screen again now (settings, more time, the clock changed). */
    val recheck = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

    /** "Turn protection off" (parent page): the accessibility service switches itself off. */
    val turnOff = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

    /** Game time left today while a Game-time app is in front (for the notification), or null. */
    @Volatile var gameLeftMin: Int? = null

    /** A part of the protection is missing (for the notification), from Health. */
    @Volatile var incomplete: Boolean = false

    /** What the DNS filter applies now, kept in step with the stored settings. */
    val filterSettings = MutableStateFlow(FilterSettings.SAFEST)

    /** Blocked lookups since the process started (the report keeps the daily count). */
    val blockedLookups = AtomicInteger()

    override fun onCreate() {
        super.onCreate()
        Notices.createChannels(this)
        repo = Repo(this, scope)
        scope.launch { repo.settings.collect { filterSettings.value = FilterSettings.from(it, bands) } }
    }

    @Suppress("UNUSED_PARAMETER")
    fun onBlocked(decision: Decision.Block, name: String) {
        blockedLookups.incrementAndGet()
    }
}
