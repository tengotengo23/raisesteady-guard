package com.raisesteady.guard.debug

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.raisesteady.guard.guard.GuardAccessibilityService
import com.raisesteady.guard.guard.Health
import com.raisesteady.guard.ui.LookLog
import com.raisesteady.guard.vpn.GuardVpnService

/**
 * Debug builds only: what is running, for the end-to-end tests (logcat lines can roll out of its buffer):
 * `am broadcast -n com.raisesteady.guard/.debug.DebugStatusReceiver` answers "a11y=true vpn=true admin=false".
 * With `--ez look true` it answers the look log (texts that overflowed, screen entries and their
 * motion, ui/LookLog); `--ez clearLook true` empties it first.
 */
class DebugStatusReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        resultCode = 1
        if (intent.getBooleanExtra("clearLook", false)) LookLog.clear()
        if (intent.getBooleanExtra("look", false)) {
            resultData = LookLog.report()
            return
        }
        resultData = "a11y=${GuardAccessibilityService.running} vpn=${GuardVpnService.state.value.running} admin=${Health.adminActive(context)}"
    }
}
