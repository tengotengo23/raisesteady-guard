package com.raisesteady.guard.debug

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import com.raisesteady.guard.GuardApp

/**
 * Debug builds only: moves Guard's clock for the end-to-end tests (bedtime), without touching the
 * device clock: `am broadcast -n com.raisesteady.guard/.debug.DebugClockReceiver --el offsetMs <ms>`.
 */
class DebugClockReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val app = context.applicationContext as GuardApp
        app.debugClockOffsetMs.set(intent.getLongExtra("offsetMs", 0L))
        Log.i("GuardDebug", "clock offset ${app.debugClockOffsetMs.get()} ms, now ${app.clock.now()}")
        app.recheck.tryEmit(Unit)
        resultCode = 1
        resultData = "offset ${app.debugClockOffsetMs.get()}"
    }
}
