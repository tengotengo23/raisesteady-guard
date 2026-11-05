package com.raisesteady.guard.guard

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.provider.Settings
import android.provider.Telephony
import android.telecom.TelecomManager

/**
 * Apps that are never limited, whatever the parent picks (design 2.4): this app, the launcher,
 * System UI, the keyboard, the default dialer and SMS app, emergency, Settings (guarded
 * separately) and the permission controller. The apps list leaves them out.
 */
object Exempt {
    fun packages(context: Context): Set<String> {
        val out = HashSet<String>()
        out += context.packageName
        out += setOf("android", "com.android.systemui", "com.android.emergency", "com.google.android.apps.safetyhub")
        out += SettingsGuard.PACKAGES
        context.packageManager.queryIntentActivities(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_HOME), PackageManager.MATCH_DEFAULT_ONLY)
            .forEach { out += it.activityInfo.packageName }
        out += transient(context)
        runCatching { context.getSystemService(TelecomManager::class.java)?.defaultDialerPackage }.getOrNull()?.let(out::add)
        runCatching { Telephony.Sms.getDefaultSmsPackage(context) }.getOrNull()?.let(out::add)
        return out
    }

    /** Windows that sit over the app in front without replacing it: the keyboard and System UI. */
    fun transient(context: Context): Set<String> {
        val out = HashSet<String>()
        out += "com.android.systemui"
        Settings.Secure.getString(context.contentResolver, Settings.Secure.DEFAULT_INPUT_METHOD)
            ?.substringBefore('/')?.takeIf { it.isNotBlank() }?.let(out::add)
        return out
    }
}
