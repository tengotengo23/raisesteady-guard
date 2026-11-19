package com.raisesteady.guard.debug

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Base64
import android.util.Log
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.update.UpdateConfig
import com.raisesteady.guard.update.Updater

/**
 * Debug builds only: points the self-updater at a test source and checks now (P2-260 end-to-end tests).
 * `am broadcast -n com.raisesteady.guard/.debug.DebugUpdateReceiver --es manifestUrl <url> --es key <base64>
 *  --es prefix <url prefix> [--ez reset true] [--ez now true]`. Release builds have no such receiver.
 */
class DebugUpdateReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val app = context.applicationContext as GuardApp
        if (intent.getBooleanExtra("reset", false)) {
            app.debugUpdateConfig = null
        } else {
            val url = intent.getStringExtra("manifestUrl")
            val key = intent.getStringExtra("key")
            if (url != null && key != null) {
                app.debugUpdateConfig = UpdateConfig(
                    manifestUrl = url,
                    publicKey = Base64.decode(key, Base64.DEFAULT),
                    urlPrefix = intent.getStringExtra("prefix") ?: UpdateConfig.PREFIX,
                )
            }
        }
        if (intent.getBooleanExtra("now", false)) Updater.checkNow(app)
        val note = "update source ${Updater.config(app).manifestUrl}, last: ${app.updates.status.value.lastNote}"
        Log.i("GuardDebug", note)
        resultCode = 1
        resultData = note
    }
}
