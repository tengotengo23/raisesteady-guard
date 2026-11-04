package com.raisesteady.guard.guard

import android.Manifest
import android.accessibilityservice.AccessibilityServiceInfo
import android.app.AppOpsManager
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.net.VpnService
import android.os.Build
import android.provider.Settings
import android.view.accessibility.AccessibilityManager
import androidx.core.content.ContextCompat
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.vpn.GuardVpnService

/**
 * Which parts of the protection work now (design 3.7, the status page's checks). Read again each
 * minute by the VPN service, so the notification stays honest.
 */
data class Health(
    val vpnConsent: Boolean,
    val vpnRunning: Boolean,
    val alwaysOn: Boolean,
    val lockdown: Boolean,
    val accessibility: Boolean,
    val usageAccess: Boolean,
    val notifications: Boolean,
    val admin: Boolean,
    /** The host name when Private DNS is set to a provider (strict mode): our filter is then skipped. */
    val privateDnsHost: String?,
    val autoTime: Boolean,
    /** "Pause app activity if unused" is off for Guard (always true before Android 11). */
    val unusedAppPauseOff: Boolean,
) {
    /** Every part that keeps limits and the guard working (the filter itself has its own notice). */
    val complete: Boolean get() = accessibility && usageAccess && admin && notifications && privateDnsHost == null

    companion object {
        fun accessibilityOn(context: Context): Boolean {
            val am = context.getSystemService(AccessibilityManager::class.java) ?: return false
            val ours = ComponentName(context, GuardAccessibilityService::class.java)
            return am.getEnabledAccessibilityServiceList(AccessibilityServiceInfo.FEEDBACK_ALL_MASK).any {
                it.resolveInfo?.serviceInfo?.let { si -> ComponentName(si.packageName, si.name) } == ours
            }
        }

        fun usageAccess(context: Context): Boolean = UsageReader.opAllowed(context, AppOpsManager.OPSTR_GET_USAGE_STATS)

        fun adminActive(context: Context): Boolean =
            context.getSystemService(DevicePolicyManager::class.java)?.isAdminActive(GuardAdminReceiver.component(context)) == true

        /** The Private DNS host of the network under our VPN, or null (off or automatic). */
        @Suppress("DEPRECATION") // allNetworks: the simplest way to see the network under the VPN
        fun privateDnsHost(context: Context): String? {
            val cm = context.getSystemService(ConnectivityManager::class.java) ?: return null
            for (n in cm.allNetworks) {
                val caps = cm.getNetworkCapabilities(n) ?: continue
                if (caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue
                cm.getLinkProperties(n)?.privateDnsServerName?.let { return it }
            }
            return null
        }

        fun read(context: Context): Health {
            val st = (context.applicationContext as GuardApp).repo.state.value
            val vpn = GuardVpnService.state.value
            return Health(
                vpnConsent = VpnService.prepare(context) == null,
                vpnRunning = vpn.running,
                alwaysOn = vpn.alwaysOn || st.alwaysOnVpn,
                lockdown = vpn.lockdown || st.lockdownVpn,
                accessibility = accessibilityOn(context),
                usageAccess = usageAccess(context),
                notifications = Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
                    ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED,
                admin = adminActive(context),
                privateDnsHost = privateDnsHost(context),
                autoTime = Settings.Global.getInt(context.contentResolver, Settings.Global.AUTO_TIME, 1) == 1,
                unusedAppPauseOff = Build.VERSION.SDK_INT < Build.VERSION_CODES.R || context.packageManager.isAutoRevokeWhitelisted,
            )
        }
    }
}
