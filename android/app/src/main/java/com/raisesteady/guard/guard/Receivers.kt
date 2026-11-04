package com.raisesteady.guard.guard

import android.app.admin.DeviceAdminReceiver
import android.content.BroadcastReceiver
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.VpnService
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.R
import com.raisesteady.guard.vpn.GuardVpnService
import com.raisesteady.guard.vpn.Notices
import kotlinx.coroutines.launch

/**
 * Device admin with no policies (design 3.6): while it is active, Android refuses to uninstall
 * the app until it is switched off on the Device admin apps page, which the guard covers.
 */
class GuardAdminReceiver : DeviceAdminReceiver() {
    override fun onDisableRequested(context: Context, intent: Intent): CharSequence =
        context.getString(R.string.admin_disable_warning)

    companion object {
        fun component(context: Context) = ComponentName(context, GuardAdminReceiver::class.java)
    }
}

/**
 * After a reboot or an update: starts the filter when the VPN permission is still given and
 * setup is done (Always-on VPN, when the parent set it, starts it by itself as well; a second
 * start does nothing). systemExempted services may start from boot (Android 15 rules).
 */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED && intent.action != Intent.ACTION_MY_PACKAGE_REPLACED) return
        val app = context.applicationContext as GuardApp
        val pending = goAsync()
        app.scope.launch {
            try {
                if (app.repo.readSettings().setupDone && VpnService.prepare(context) == null) GuardVpnService.start(context)
            } finally {
                pending.finish()
            }
        }
    }
}

/** "Lock now" on the "Settings are unlocked" notice: the guarded pages need the PIN again. */
class LockNowReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val app = context.applicationContext as GuardApp
        val pending = goAsync()
        app.scope.launch {
            try {
                app.repo.updateState { it.copy(settingsUnlockedUntil = 0) }
                Notices.clearUnlocked(context)
                app.recheck.tryEmit(Unit)
            } finally {
                pending.finish()
            }
        }
    }
}

/**
 * A new app (registered by the accessibility service while it runs: Android no longer delivers
 * this broadcast to receivers in the manifest). Its level is its default (AppCatalog: a game
 * starts on Game time, a known bypass app Blocked), read fresh, so it applies at once.
 */
class PackageReceiver(private val onAdded: (String) -> Unit) : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.getBooleanExtra(Intent.EXTRA_REPLACING, false)) return
        intent.data?.schemeSpecificPart?.let(onAdded)
    }
}
