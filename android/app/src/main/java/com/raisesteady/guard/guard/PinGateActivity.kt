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
import com.raisesteady.guard.ui.child.GateScreen
import com.raisesteady.guard.ui.theme.GuardTheme
import com.raisesteady.guard.vpn.Notices
import kotlinx.coroutines.flow.first
import java.time.Instant

/**
 * The settings cover (design 3.6, 4.3): a page that changes how Guard protects the device needs
 * the parent PIN. The right PIN opens these pages for 10 minutes ("Lock now" on the notice
 * closes them again, as does Lock now on the parent page); "Back" goes to the home screen. The
 * screen itself: ui/child/Child.kt.
 */
class PinGateActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() = goHome()
        })
        setContent { GuardTheme { GateScreen(onUnlock = ::unlock, onBack = ::goHome) } }
    }

    /** Null when unlocked; otherwise the message for the PIN field. */
    private suspend fun unlock(pin: String): String? {
        ParentPin.check(this, pin)?.let { return it }
        val app = application as GuardApp
        val until = app.clock.now().toEpochMilli() + SettingsGuard.UNLOCK_MS
        val next = app.repo.updateState { it.copy(settingsUnlockedUntil = until) }
        app.repo.state.first { it == next }
        Notices.settingsUnlocked(this, ParentPin.time(this, Instant.ofEpochMilli(until)), SettingsGuard.UNLOCK_MS)
        finish() // back to the page underneath, now open
        return null
    }

    private fun goHome() {
        startActivity(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        finish()
    }

    companion object {
        fun intent(context: Context): Intent = Intent(context, PinGateActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_NO_ANIMATION)
    }
}
