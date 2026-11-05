package com.raisesteady.guard.debug

import android.app.Activity
import android.net.VpnService
import android.os.Bundle
import android.util.Log
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.data.State
import com.raisesteady.guard.dns.FilterSettings
import com.raisesteady.guard.pin.Pin
import com.raisesteady.guard.vpn.GuardVpnService
import kotlinx.coroutines.runBlocking

/**
 * Debug builds only. Sets the parent's settings and starts the site filter for the end-to-end
 * tests, before the setup screens exist (W6):
 * `am start -n com.raisesteady.guard/.debug.DebugFilterActivity --es band 9-12 --ez familyDns true`
 * Optional: `--ei gameMinutes 1` (debug builds allow 1 minute), `--es pin 2468`, `--ez resetState true`
 * (no pause, more time, PIN lock or settings unlock), `--ez setupDone false` (default true: the
 * guard acts only after setup), `--ez bedtimeOn false` (tests that run at any hour), `--ez fresh true` (everything as after a new install; the filter
 * is not started). VPN consent is pre-approved with `appops set com.raisesteady.guard ACTIVATE_VPN allow`.
 */
class DebugFilterActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = application as GuardApp
        val band = app.bands.byId(intent.getStringExtra("band") ?: "9-12")
        val newPin = intent.getStringExtra("pin")?.let { Pin.hash(it) }
        if (intent.getBooleanExtra("fresh", false)) {
            runBlocking {
                app.repo.updateState { State() }
                app.repo.updateSettings { Settings() }
            }
            app.filterSettings.value = FilterSettings.SAFEST
            Log.i("GuardDebug", "settings: fresh")
            finish()
            return
        }
        val stored = runBlocking {
            if (intent.getBooleanExtra("resetState", false)) app.repo.updateState { State() }
            app.repo.updateSettings {
                val base = Settings.forBand(band)
                base.copy(
                    pin = newPin ?: it.pin,
                    familyDns = intent.getBooleanExtra("familyDns", true),
                    topicFilter = intent.getBooleanExtra("topicFilter", true),
                    gameMinutes = intent.getIntExtra("gameMinutes", base.gameMinutes),
                    setupDone = intent.getBooleanExtra("setupDone", true),
                    bedtimeOn = intent.getBooleanExtra("bedtimeOn", base.bedtimeOn),
                )
            }
        }
        app.filterSettings.value = FilterSettings.from(stored, app.bands)
        if (VpnService.prepare(this) == null) GuardVpnService.start(this) else Log.w("GuardDebug", "VPN consent missing")
        Log.i("GuardDebug", "settings: band ${stored.bandId}, game ${stored.gameMinutes} min, pin ${stored.pin != null}")
        finish()
    }
}
