package com.raisesteady.guard.debug

import android.app.Activity
import android.os.Bundle
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.guard.BlockActivity
import com.raisesteady.guard.guard.PinGateActivity
import com.raisesteady.guard.rules.Reason

/**
 * Debug builds only: opens a child screen for the look tests without waiting for a limit:
 * `am start -n com.raisesteady.guard/.debug.DebugShowActivity --es screen block --es reason BEDTIME`
 * (`--es pkg <app>`, default the test game) or `--es screen gate`.
 */
class DebugShowActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = application as GuardApp
        when (intent.getStringExtra("screen")) {
            "gate" -> startActivity(PinGateActivity.intent(this))
            else -> {
                val reason = runCatching { Reason.valueOf(intent.getStringExtra("reason") ?: "") }.getOrDefault(Reason.TIME_UP)
                val until = if (reason == Reason.TIME_UP || reason == Reason.BLOCKED) 0L else app.clock.now().toEpochMilli() + 60 * 60_000L
                startActivity(BlockActivity.intent(this, intent.getStringExtra("pkg") ?: "com.raisesteady.guard.e2e", reason, until))
            }
        }
        finish()
    }
}
