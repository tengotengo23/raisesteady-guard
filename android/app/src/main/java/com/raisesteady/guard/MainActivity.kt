package com.raisesteady.guard

import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.raisesteady.guard.ui.Routes
import com.raisesteady.guard.ui.parent.AppsScreen
import com.raisesteady.guard.ui.parent.OffScreen
import com.raisesteady.guard.ui.parent.ParentScreen
import com.raisesteady.guard.ui.parent.PinScreen
import com.raisesteady.guard.ui.setup.STEPS
import com.raisesteady.guard.ui.setup.SetupFlow
import com.raisesteady.guard.ui.status.StatusScreen
import com.raisesteady.guard.ui.theme.GuardTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking

/**
 * Single activity: the setup (until it is done), then the status page; "For the parent" (PIN)
 * opens the parent page and its apps list. Routes: "setup:<step>", "status", "pin", "parent",
 * "apps", "off" (rules in ui/Routes). The parent session is short-lived: it ends when the app
 * leaves the screen, is never restored, and closes after 5 minutes without a touch. Debug builds
 * take a start route for the look tests: `--es debugScreen <route>`.
 */
class MainActivity : ComponentActivity() {

    private var route by mutableStateOf(Routes.STATUS)
    private var lastTouchMs = SystemClock.elapsedRealtime()

    private val app get() = application as GuardApp
    private val setupDone get() = app.repo.settings.value.setupDone

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        // The stored settings decide the first screen (a small file; read before the first frame).
        val done = runBlocking { app.repo.readSettings().setupDone }
        route = debugRoute(intent) ?: Routes.restored(savedInstanceState?.getString(KEY_ROUTE), done)
        setContent {
            GuardTheme {
                val go: (String) -> Unit = { wanted -> route = Routes.next(route, wanted, setupDone) }
                LaunchedEffect(route) {
                    while (Routes.privileged(route, setupDone)) {
                        delay(IDLE_CHECK_MS)
                        if (Routes.idle(route, setupDone, lastTouchMs, SystemClock.elapsedRealtime())) route = Routes.home(setupDone)
                    }
                }
                BackHandler(enabled = route != Routes.STATUS && route != Routes.FIRST_STEP) {
                    go(
                        when {
                            Routes.isSetup(route) -> "setup:${(route.removePrefix("setup:").toIntOrNull() ?: 2) - 1}"
                            route == Routes.APPS -> Routes.PARENT
                            route == Routes.OFF -> Routes.FIRST_STEP
                            else -> Routes.STATUS
                        },
                    )
                }
                val r = route
                when {
                    Routes.isSetup(r) -> SetupFlow(
                        step = r.removePrefix("setup:").toIntOrNull()?.coerceIn(1, STEPS) ?: 1,
                        onStep = { s -> go("setup:${s.coerceIn(1, STEPS)}") },
                        onDone = { go(Routes.STATUS) },
                    )
                    r == Routes.PIN -> PinScreen(onOk = { go(Routes.PARENT) }, onCancel = { go(Routes.STATUS) })
                    r == Routes.PARENT -> ParentScreen(onApps = { go(Routes.APPS) }, onClose = { go(Routes.STATUS) }, onOff = { go(Routes.OFF) })
                    r == Routes.APPS -> AppsScreen(onBack = { go(Routes.PARENT) })
                    r == Routes.OFF -> OffScreen(onSetup = { go(Routes.FIRST_STEP) })
                    else -> StatusScreen(onParent = {
                        lastTouchMs = SystemClock.elapsedRealtime()
                        go(Routes.PIN)
                    })
                }
            }
        }
    }

    override fun onUserInteraction() {
        super.onUserInteraction()
        lastTouchMs = SystemClock.elapsedRealtime()
    }

    /** Leaving the screen (home, another app, the screen off) ends the parent session. */
    override fun onStop() {
        super.onStop()
        route = Routes.onLeave(route, setupDone)
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putString(KEY_ROUTE, route) // Routes.restored never gives a PIN-gated route back
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        debugRoute(intent)?.let { route = it }
    }

    /** Debug builds only: the route the look tests open ("setup:8", "status", "parent", "apps"). */
    private fun debugRoute(intent: Intent?): String? =
        if (BuildConfig.DEBUG) intent?.getStringExtra("debugScreen") else null

    private companion object {
        const val KEY_ROUTE = "route"
        const val IDLE_CHECK_MS = 10_000L
    }
}
