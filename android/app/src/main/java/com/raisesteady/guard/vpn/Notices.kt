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

    fun protectionOn(context: Context): Notification =
        NotificationCompat.Builder(context, CHANNEL_PROTECTION)
            .setSmallIcon(R.drawable.ic_stat_guard)
            .setContentTitle(context.getString(R.string.notif_on_title))
            .setContentText(context.getString(R.string.notif_on_text))
            .setContentIntent(openApp(context))
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .build()

    /** "The site filter is off. Ask a parent to open RaiseSteady Guard." */
    fun filterOff(context: Context) = notify(context, ID_FILTER_OFF, notice(context, R.string.notif_filter_off))

    fun clearFilterOff(context: Context) = context.getSystemService(NotificationManager::class.java).cancel(ID_FILTER_OFF)

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
