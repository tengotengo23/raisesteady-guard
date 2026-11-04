package com.raisesteady.guard.debug

import android.app.Activity
import android.net.VpnService
import android.os.Bundle
import android.util.Log
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.dns.FilterSettings
import com.raisesteady.guard.vpn.GuardVpnService
import kotlinx.coroutines.runBlocking

/**
 * Debug builds only. Starts the site filter for the end-to-end tests, before the setup screens exist:
 * `am start -n com.raisesteady.guard/.debug.DebugFilterActivity --es band 9-12 --ez familyDns true`
 * (VPN consent pre-approved with `appops set com.raisesteady.guard ACTIVATE_VPN allow`).
 */
class DebugFilterActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = application as GuardApp
        val band = app.bands.byId(intent.getStringExtra("band") ?: "9-12")
        val stored = runBlocking {
            app.repo.updateSettings {
                Settings.forBand(band).copy(
                    pin = it.pin,
                    familyDns = intent.getBooleanExtra("familyDns", true),
                    topicFilter = intent.getBooleanExtra("topicFilter", true),
                )
            }
        }
        app.filterSettings.value = FilterSettings.from(stored, app.bands)
        if (VpnService.prepare(this) == null) GuardVpnService.start(this) else Log.w("GuardDebug", "VPN consent missing")
        finish()
    }
}
