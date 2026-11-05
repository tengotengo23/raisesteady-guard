package com.raisesteady.guard.ui

import android.app.admin.DevicePolicyManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.os.Build
import android.provider.Settings
import androidx.compose.runtime.Composable
import androidx.compose.runtime.State
import androidx.compose.runtime.produceState
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.core.content.IntentCompat
import androidx.core.graphics.drawable.toBitmap
import androidx.core.net.toUri
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.repeatOnLifecycle
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.R
import com.raisesteady.guard.guard.Exempt
import com.raisesteady.guard.guard.GuardAccessibilityService
import com.raisesteady.guard.guard.GuardAdminReceiver
import com.raisesteady.guard.guard.Health
import com.raisesteady.guard.guard.LockNowReceiver
import com.raisesteady.guard.guard.SettingsGuard
import com.raisesteady.guard.vpn.GuardVpnService
import com.raisesteady.guard.vpn.Notices
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.text.Collator
import java.time.Instant

val Context.guardApp: GuardApp get() = applicationContext as GuardApp

/** Health, read again each second while the screen is shown (a permission step moves on when its part turns on). */
@Composable
fun rememberHealth(): State<Health?> {
    val ctx = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    return produceState<Health?>(null, lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            while (true) {
                value = withContext(Dispatchers.Default) { Health.read(ctx) }
                delay(1_000)
            }
        }
    }
}

/** One installed app with a launcher icon (the apps list). */
data class InstalledApp(val pkg: String, val label: String, val isGame: Boolean, val icon: ImageBitmap?)

/** The apps on this device with a launcher icon, without the ones that are never limited (Exempt). */
@Composable
fun rememberInstalledApps(): State<List<InstalledApp>?> {
    val ctx = LocalContext.current
    return produceState<List<InstalledApp>?>(null) {
        value = withContext(Dispatchers.IO) { installedApps(ctx) }
    }
}

fun installedApps(ctx: Context): List<InstalledApp> {
    val pm = ctx.packageManager
    val exempt = Exempt.packages(ctx)
    val size = (40 * ctx.resources.displayMetrics.density).toInt()
    val collator = Collator.getInstance(ctx.resources.configuration.locales[0])
    return pm.queryIntentActivities(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER), 0)
        .map { it.activityInfo.applicationInfo }
        .distinctBy { it.packageName }
        .filter { it.packageName !in exempt }
        .map { ai ->
            InstalledApp(
                pkg = ai.packageName,
                label = pm.getApplicationLabel(ai).toString(),
                isGame = ai.category == ApplicationInfo.CATEGORY_GAME,
                icon = runCatching { pm.getApplicationIcon(ai).toBitmap(size, size).asImageBitmap() }.getOrNull(),
            )
        }
        .sortedWith { a, b -> collator.compare(a.label, b.label) }
}

fun appLabel(ctx: Context, pkg: String): String =
    runCatching { ctx.packageManager.getApplicationLabel(ctx.packageManager.getApplicationInfo(pkg, 0)).toString() }.getOrDefault(pkg)

/** Opens a page in Settings; the first intent that the device has wins. */
fun open(ctx: Context, vararg intents: Intent) {
    for (i in intents) {
        try {
            ctx.startActivity(i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            return
        } catch (_: ActivityNotFoundException) {
        } catch (_: SecurityException) {
        }
    }
}

object Pages {
    fun appInfo(ctx: Context) = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, "package:${ctx.packageName}".toUri())

    fun usageAccess(ctx: Context) = arrayOf(
        Intent(Settings.ACTION_USAGE_ACCESS_SETTINGS, "package:${ctx.packageName}".toUri()),
        Intent(Settings.ACTION_USAGE_ACCESS_SETTINGS),
    )

    /** Our own page in Accessibility (Android 12+ opens it for the app itself), else the Accessibility list. */
    fun accessibility(ctx: Context): Array<Intent> {
        val component = "${ctx.packageName}/${GuardAccessibilityService::class.java.name}"
        val list = Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) return arrayOf(list)
        return arrayOf(Intent("android.settings.ACCESSIBILITY_DETAILS_SETTINGS").putExtra(Intent.EXTRA_COMPONENT_NAME, component), list)
    }

    fun addAdmin(ctx: Context) = Intent(DevicePolicyManager.ACTION_ADD_DEVICE_ADMIN)
        .putExtra(DevicePolicyManager.EXTRA_DEVICE_ADMIN, GuardAdminReceiver.component(ctx))
        .putExtra(DevicePolicyManager.EXTRA_ADD_EXPLANATION, ctx.getString(R.string.admin_description))

    fun vpn() = Intent(Settings.ACTION_VPN_SETTINGS)

    fun notifications(ctx: Context) = Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, ctx.packageName)

    fun privateDns() = arrayOf(Intent("android.settings.NETWORK_AND_INTERNET_SETTINGS"), Intent(Settings.ACTION_WIRELESS_SETTINGS))

    fun dateTime() = Intent(Settings.ACTION_DATE_SETTINGS)

    fun unusedApps(ctx: Context) = IntentCompat.createManageUnusedAppRestrictionsIntent(ctx, ctx.packageName)

    fun battery() = Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)

    fun readme(ctx: Context) = Intent(Intent.ACTION_VIEW, ctx.getString(R.string.readme_url).toUri())

    /** Makers whose phones are known to stop background apps (battery link on the last setup step). */
    val batteryKillers = setOf("xiaomi", "redmi", "poco", "huawei", "honor", "oppo", "realme", "vivo", "oneplus", "samsung", "meizu", "asus")
}

/**
 * The parent, already past the PIN in this app, opens a guarded Settings page ("Fix"): the pages
 * are unlocked for 10 minutes first, with the "Settings are unlocked" notice and its Lock now.
 */
suspend fun unlockSettings(ctx: Context) {
    val app = ctx.guardApp
    val until = app.clock.now().toEpochMilli() + SettingsGuard.UNLOCK_MS
    app.repo.updateState { it.copy(settingsUnlockedUntil = until) }
    Notices.settingsUnlocked(ctx, Fmt.time(ctx, Instant.ofEpochMilli(until)), SettingsGuard.UNLOCK_MS)
}

/** "Lock now" on the parent page (director's decision D2), the same as on the notice. */
fun lockSettingsNow(ctx: Context) = LockNowReceiver.lockNow(ctx)

/**
 * "Turn protection off" (design 3.6): stops the filter, removes the device admin, switches the
 * accessibility service off and forgets the settings (a fresh setup can follow).
 */
suspend fun turnProtectionOff(ctx: Context) {
    val app = ctx.guardApp
    GuardVpnService.stop(ctx)
    runCatching { ctx.getSystemService(DevicePolicyManager::class.java)?.removeActiveAdmin(GuardAdminReceiver.component(ctx)) }
    app.turnOff.tryEmit(Unit)
    app.repo.updateSettings { com.raisesteady.guard.data.Settings() }
    app.repo.updateState { com.raisesteady.guard.data.State() }
    withContext(Dispatchers.Main) {
        listOf(Notices.ID_FILTER_OFF, Notices.ID_LIMITS_OFF, Notices.ID_FIVE_MINUTES, Notices.ID_UNLOCKED, Notices.ID_LIMITS_ONLY).forEach { Notices.cancel(ctx, it) }
    }
}

