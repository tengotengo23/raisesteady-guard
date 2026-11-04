package com.raisesteady.guard.vpn

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.raisesteady.guard.MainActivity
import com.raisesteady.guard.R

/** The persistent "on" notification and the calm notices (design 3.7, 4.5). */
object Notices {
    const val CHANNEL_PROTECTION = "protection"
    const val CHANNEL_NOTICES = "notices"
    const val ID_PROTECTION = 1
    const val ID_FILTER_OFF = 2
    const val ID_LIMITS_OFF = 3
    const val ID_FIVE_MINUTES = 4
    const val ID_UNLOCKED = 5

    fun createChannels(context: Context) {
        val nm = context.getSystemService(NotificationManager::class.java)
        val protection = NotificationChannel(CHANNEL_PROTECTION, context.getString(R.string.channel_protection), NotificationManager.IMPORTANCE_LOW)
            .apply { setShowBadge(false) }
        val notices = NotificationChannel(CHANNEL_NOTICES, context.getString(R.string.channel_notices), NotificationManager.IMPORTANCE_DEFAULT)
            .apply { setSound(null, null) }
        nm.createNotificationChannels(listOf(protection, notices))
    }

    fun openApp(context: Context): PendingIntent = PendingIntent.getActivity(
        context, 0,
        Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    /** The text of the persistent notification (design 4.5). */
    fun protectionText(context: Context): String {
        val app = context.applicationContext as? com.raisesteady.guard.GuardApp
        val left = app?.gameLeftMin
        return when {
            app?.incomplete == true -> context.getString(R.string.notif_incomplete)
            left != null -> context.getString(R.string.notif_game_left, left)
            else -> context.getString(R.string.notif_on_text)
        }
    }

    fun protectionOn(context: Context): Notification =
        NotificationCompat.Builder(context, CHANNEL_PROTECTION)
            .setSmallIcon(R.drawable.ic_stat_guard)
            .setContentTitle(context.getString(R.string.notif_on_title))
            .setContentText(protectionText(context))
            .setContentIntent(openApp(context))
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .build()

    /** "The site filter is off. Ask a parent to open RaiseSteady Guard." */
    fun filterOff(context: Context) = notify(context, ID_FILTER_OFF, notice(context, R.string.notif_filter_off))

    fun clearFilterOff(context: Context) = cancel(context, ID_FILTER_OFF)

    /** Rewrites the persistent notification when the filter runs (it belongs to the VPN service). */
    fun refreshProtection(context: Context) {
        if (GuardVpnService.state.value.running) notify(context, ID_PROTECTION, protectionOn(context))
    }

    /** "Time limits are off. Ask a parent to open RaiseSteady Guard." */
    fun limitsOff(context: Context) = notify(context, ID_LIMITS_OFF, notice(context, R.string.notif_limits_off))

    fun clearLimitsOff(context: Context) = cancel(context, ID_LIMITS_OFF)

    /** "5 minutes of game time left today." */
    fun fiveMinutes(context: Context) = notify(context, ID_FIVE_MINUTES, notice(context, R.string.notif_five_minutes))

    /** "Settings are unlocked until {time}." with "Lock now". */
    fun settingsUnlocked(context: Context, until: String, timeoutMs: Long) {
        val lock = PendingIntent.getBroadcast(
            context, 1,
            Intent(context, com.raisesteady.guard.guard.LockNowReceiver::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val text = context.getString(R.string.unlocked_until, until)
        val n = NotificationCompat.Builder(context, CHANNEL_NOTICES)
            .setSmallIcon(R.drawable.ic_stat_guard)
            .setContentTitle(context.getString(R.string.app_name))
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setOngoing(true)
            .setSilent(true)
            // Tapping the notice locks too: the button can be folded away in a group.
            .setContentIntent(
                PendingIntent.getActivity(
                    context, 2, Intent(context, com.raisesteady.guard.guard.LockNowActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                    PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
                ),
            )
            .setTimeoutAfter(timeoutMs)
            .setGroup("com.raisesteady.guard.unlocked") // its own group: never folded in with other notices
            .addAction(0, context.getString(R.string.lock_now), lock)
            .build()
        notify(context, ID_UNLOCKED, n)
    }

    fun clearUnlocked(context: Context) = cancel(context, ID_UNLOCKED)

    fun cancel(context: Context, id: Int) = context.getSystemService(NotificationManager::class.java).cancel(id)

    private fun notice(context: Context, text: Int): Notification =
        NotificationCompat.Builder(context, CHANNEL_NOTICES)
            .setSmallIcon(R.drawable.ic_stat_guard)
            .setContentTitle(context.getString(R.string.app_name))
            .setContentText(context.getString(text))
            .setStyle(NotificationCompat.BigTextStyle().bigText(context.getString(text)))
            .setContentIntent(openApp(context))
            .setAutoCancel(true)
            .build()

    fun notify(context: Context, id: Int, n: Notification) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) {
            context.getSystemService(NotificationManager::class.java).notify(id, n)
        }
    }
}
