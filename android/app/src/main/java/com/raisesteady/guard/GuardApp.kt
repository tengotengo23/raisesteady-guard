package com.raisesteady.guard

import android.app.Application
import com.raisesteady.guard.data.AgeBands
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
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import java.util.concurrent.atomic.AtomicInteger

/** The application: the store, shared data, the filter's settings and the notification channels. */
class GuardApp : Application() {
    /** Lives as long as the process. */
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    /** The block lists and helplines, loaded once from the assets (about 1.4 MB). */
    val data: GuardData by lazy { GuardData.load { assets.open(it) } }

    val bands: AgeBands by lazy { AgeBands.parse(assets.open("age-bands.json").use { it.reader().readText() }) }

    lateinit var repo: Repo
        private set

    /** Debug builds may move it for tests (W5); release builds use the device clock as it is. */
    val clock: Clock = DeviceClock()

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
