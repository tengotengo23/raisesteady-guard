package com.raisesteady.guard.guard

import android.content.Context
import android.text.format.DateFormat
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.R
import com.raisesteady.guard.pin.Pin
import java.time.Instant
import java.util.Date

/** One PIN try with the stored lockout (five wrong tries lock 15 minutes, kept in state.json). */
object ParentPin {
    /** Null when right; otherwise the message to show under the field. */
    suspend fun check(context: Context, pin: String): String? {
        val app = context.applicationContext as GuardApp
        val now = app.clock.now().toEpochMilli()
        val rec = app.repo.readSettings().pin
        var result: Pin.Result? = null
        app.repo.updateState { st ->
            val (r, lock) = Pin.check(pin, rec, st.pinLock, now)
            result = r
            st.copy(pinLock = lock)
        }
        val r = result!!
        return when {
            r.ok -> null
            r.locked -> context.getString(R.string.pin_locked, time(context, Instant.ofEpochMilli(r.until)))
            else -> context.getString(R.string.pin_wrong)
        }
    }

    /** A clock time in the device's format (12 or 24 hours). */
    fun time(context: Context, t: Instant): String = DateFormat.getTimeFormat(context).format(Date(t.toEpochMilli()))
}
