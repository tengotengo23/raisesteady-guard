package com.raisesteady.guard.guard

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.R
import com.raisesteady.guard.rules.Reason
import com.raisesteady.guard.ui.child.BlockScreen
import com.raisesteady.guard.ui.theme.GuardTheme
import kotlinx.coroutines.flow.first
import java.time.Instant

/**
 * The closed-app screen (design 4.3): why the app is closed and how to ask a parent. "For the
 * parent" takes the PIN and gives 15 or 30 more minutes (not for Blocked apps). Leaving it goes
 * to the home screen. The screen itself: ui/child/Child.kt.
 */
class BlockActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() = goHome()
        })
        show(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        show(intent)
    }

    private fun show(intent: Intent) {
        val pkg = intent.getStringExtra(EXTRA_PACKAGE) ?: return finish()
        val reason = runCatching { Reason.valueOf(intent.getStringExtra(EXTRA_REASON) ?: "") }.getOrDefault(Reason.BLOCKED)
        val until = intent.getLongExtra(EXTRA_UNTIL, 0L)
        val label = runCatching { packageManager.getApplicationLabel(packageManager.getApplicationInfo(pkg, 0)).toString() }.getOrDefault(pkg)
        val untilText = if (until > 0) ParentPin.time(this, Instant.ofEpochMilli(until)) else ""
        val reasonText = when (reason) {
            Reason.TIME_UP -> getString(R.string.reason_time_up)
            Reason.BEDTIME -> getString(R.string.reason_bedtime, untilText)
            Reason.HOMEWORK -> getString(R.string.reason_homework, untilText)
            Reason.PAUSED -> getString(R.string.reason_paused, untilText)
            Reason.BLOCKED -> getString(R.string.reason_blocked)
        }
        setContent {
            GuardTheme {
                BlockScreen(
                    title = getString(R.string.block_title, label),
                    reason = reason,
                    reasonText = reasonText,
                    onHome = ::goHome,
                    onMoreTime = { pin, minutes -> moreTime(pkg, pin, minutes) },
                )
            }
        }
    }

    /** Null when given; otherwise the message for the PIN field. */
    private suspend fun moreTime(pkg: String, pin: String, minutes: Int): String? {
        ParentPin.check(this, pin)?.let { return it }
        val app = application as GuardApp
        val now = app.clock.now().toEpochMilli()
        val next = app.repo.updateState { it.withMoreTime(minutes, now) }
        app.repo.state.first { it == next } // the guard reads the new state before the app comes back
        packageManager.getLaunchIntentForPackage(pkg)?.let { startActivity(it.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
        finish()
        return null
    }

    private fun goHome() {
        startActivity(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        finish()
    }

    companion object {
        private const val EXTRA_PACKAGE = "pkg"
        private const val EXTRA_REASON = "reason"
        private const val EXTRA_UNTIL = "until"

        fun intent(context: Context, pkg: String, reason: Reason, until: Long): Intent =
            Intent(context, BlockActivity::class.java)
                .putExtra(EXTRA_PACKAGE, pkg)
                .putExtra(EXTRA_REASON, reason.name)
                .putExtra(EXTRA_UNTIL, until)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_NO_ANIMATION)
    }
}
