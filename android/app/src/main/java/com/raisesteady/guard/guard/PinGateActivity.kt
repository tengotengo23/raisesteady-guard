package com.raisesteady.guard.guard

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.R
import com.raisesteady.guard.ui.components.GuardCard
import com.raisesteady.guard.ui.components.MainButton
import com.raisesteady.guard.ui.components.ParentPinField
import com.raisesteady.guard.ui.components.SecondButton
import com.raisesteady.guard.ui.theme.GuardDimens
import com.raisesteady.guard.ui.theme.GuardTheme
import com.raisesteady.guard.ui.theme.LocalGuardColors
import com.raisesteady.guard.vpn.Notices
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import java.time.Instant

/**
 * The settings cover (design 3.6, 4.3): a page that changes how Guard protects the device needs
 * the parent PIN. The right PIN opens these pages for 10 minutes ("Lock now" on the notice
 * closes them again); "Back" goes to the home screen. Plain layout: desk C finishes the look (W6).
 */
class PinGateActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() = goHome()
        })
        setContent {
            GuardTheme {
                val c = LocalGuardColors.current
                val scope = rememberCoroutineScope()
                var pin by remember { mutableStateOf("") }
                var error by remember { mutableStateOf<String?>(null) }
                Surface(Modifier.fillMaxSize(), color = c.bg) {
                    Column(
                        Modifier.safeDrawingPadding().verticalScroll(rememberScrollState()).padding(GuardDimens.gap),
                        verticalArrangement = Arrangement.spacedBy(GuardDimens.gap),
                    ) {
                        GuardCard {
                            Text(
                                stringResource(R.string.gate_title), style = MaterialTheme.typography.headlineMedium, color = c.text,
                                modifier = Modifier.semantics { heading() },
                            )
                            Text(stringResource(R.string.gate_text), style = MaterialTheme.typography.bodyLarge, color = c.text)
                            ParentPinField(pin, { pin = it; error = null }, stringResource(R.string.pin_label), error)
                        }
                        MainButton(stringResource(R.string.gate_unlock), {
                            scope.launch {
                                error = unlock(pin)
                                if (error != null) pin = ""
                            }
                        })
                        SecondButton(stringResource(R.string.gate_back), ::goHome)
                    }
                }
            }
        }
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
