package com.raisesteady.guard.ui

import android.content.Context
import android.text.format.DateFormat
import com.raisesteady.guard.R
import com.raisesteady.guard.data.Window
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.util.Calendar
import java.util.Date

/** Times and durations as the device shows them (12 or 24 hours, the device's language). */
object Fmt {
    fun time(context: Context, t: Instant): String = DateFormat.getTimeFormat(context).format(Date(t.toEpochMilli()))

    /** "21:00" or "9:00 PM" for a stored "HH:MM". */
    fun clock(context: Context, hm: String): String {
        val m = Window.minutes(hm) ?: return hm
        val cal = Calendar.getInstance().apply {
            set(Calendar.HOUR_OF_DAY, m / 60)
            set(Calendar.MINUTE, m % 60)
        }
        return DateFormat.getTimeFormat(context).format(cal.time)
    }

    fun hm(hour: Int, minute: Int) = "%02d:%02d".format(hour, minute)

    fun minutes(context: Context, min: Int): String =
        if (min >= 60) context.getString(R.string.dur_hm, min / 60, min % 60) else context.getString(R.string.dur_m, min)

    /** "Mon 3 Nov" in the device's language. */
    fun day(context: Context, d: LocalDate, zone: ZoneId = ZoneId.systemDefault()): String {
        val date = Date.from(d.atStartOfDay(zone).toInstant())
        val pattern = DateFormat.getBestDateTimePattern(context.resources.configuration.locales[0], "EEEdMMM")
        return android.text.format.DateFormat.format(pattern, date).toString()
    }
}
